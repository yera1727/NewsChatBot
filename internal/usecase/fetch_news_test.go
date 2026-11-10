package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"PetProjectClaude/internal/domain"
)

// Главный плюс Clean Architecture: use case тестируется без интернета и БД.
// Подставляем «фейковые» реализации интерфейсов.

type fakeFetcher struct {
	feeds map[string]domain.Feed
	errs  map[string]error
}

func (f *fakeFetcher) Fetch(_ context.Context, url string) (domain.Feed, error) {
	return f.feeds[url], f.errs[url]
}

type fakeSources struct {
	list   []domain.Source
	titles map[int64]string // какие названия выставил use case
}

func (f *fakeSources) ActiveSources(context.Context) ([]domain.Source, error) {
	return f.list, nil
}

func (f *fakeSources) UpdateSourceTitle(_ context.Context, id int64, title string) error {
	if f.titles == nil {
		f.titles = make(map[int64]string)
	}
	f.titles[id] = title
	return nil
}

// fakeRepo ведёт себя как настоящая БД: статью с уже известной ссылкой не сохраняет.
type fakeRepo struct {
	saved []domain.Article
	seen  map[string]bool
}

func (f *fakeRepo) Save(_ context.Context, articles []domain.Article) (int, error) {
	if f.seen == nil {
		f.seen = make(map[string]bool)
	}
	added := 0
	for _, a := range articles {
		if f.seen[a.Link] {
			continue
		}
		f.seen[a.Link] = true
		f.saved = append(f.saved, a)
		added++
	}
	return added, nil
}

func TestFetchNews_OneFeedFails_OthersStillSaved(t *testing.T) {
	fetcher := &fakeFetcher{
		feeds: map[string]domain.Feed{
			"good": {Articles: []domain.Article{{Link: "l1"}, {Link: "l2"}}},
		},
		errs: map[string]error{"bad": errors.New("сайт недоступен")},
	}
	sources := &fakeSources{list: []domain.Source{{ID: 1, URL: "bad"}, {ID: 2, URL: "good"}}}
	repo := &fakeRepo{}

	added, err := NewFetchNews(fetcher, sources, repo).Run(context.Background())

	if err == nil {
		t.Error("ожидали ошибку от ленты bad, получили nil")
	}
	if added != 2 || len(repo.saved) != 2 {
		t.Errorf("ожидали 2 новые новости, получили added=%d saved=%d", added, len(repo.saved))
	}
}

func TestFetchNews_ArticlesLinkedToSource_TitleUpdated(t *testing.T) {
	fetcher := &fakeFetcher{feeds: map[string]domain.Feed{
		"habr-url": {Title: "Хабр", Articles: []domain.Article{{Link: "l1"}}},
	}}
	sources := &fakeSources{list: []domain.Source{{ID: 7, URL: "habr-url"}}}
	repo := &fakeRepo{}

	if _, err := NewFetchNews(fetcher, sources, repo).Run(context.Background()); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	if sources.titles[7] != "Хабр" {
		t.Errorf("название источника 7 должно обновиться на «Хабр», получили %q", sources.titles[7])
	}
	if got := repo.saved[0].Source; got.ID != 7 || got.Title != "Хабр" {
		t.Errorf("статья должна быть привязана к источнику 7 «Хабр», получили %+v", got)
	}
}

func TestFetchNews_SameTitle_NotUpdated(t *testing.T) {
	fetcher := &fakeFetcher{feeds: map[string]domain.Feed{"u": {Title: "Хабр"}}}
	sources := &fakeSources{list: []domain.Source{{ID: 1, URL: "u", Title: "Хабр"}}}

	if _, err := NewFetchNews(fetcher, sources, &fakeRepo{}).Run(context.Background()); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(sources.titles) != 0 {
		t.Errorf("название не изменилось — лишний UPDATE не нужен, а он был: %v", sources.titles)
	}
}

func TestFetchNews_SecondRunFindsNothingNew(t *testing.T) {
	fetcher := &fakeFetcher{feeds: map[string]domain.Feed{
		"feed": {Articles: []domain.Article{{Link: "l1"}, {Link: "l2"}}},
	}}
	uc := NewFetchNews(fetcher, &fakeSources{list: []domain.Source{{ID: 1, URL: "feed"}}}, &fakeRepo{})

	if added, _ := uc.Run(context.Background()); added != 2 {
		t.Errorf("первый запуск: ожидали 2 новые, получили %d", added)
	}
	if added, _ := uc.Run(context.Background()); added != 0 {
		t.Errorf("второй запуск: ожидали 0 новых, получили %d", added)
	}
}

// slowFetcher имитирует медленную сеть: каждая загрузка занимает delay.
type slowFetcher struct {
	delay time.Duration
}

func (s *slowFetcher) Fetch(_ context.Context, url string) (domain.Feed, error) {
	time.Sleep(s.delay)
	return domain.Feed{Articles: []domain.Article{{Link: url}}}, nil
}

func TestFetchNews_FeedsFetchedInParallel(t *testing.T) {
	const delay = 200 * time.Millisecond
	var list []domain.Source
	for i, u := range []string{"a", "b", "c", "d", "e"} {
		list = append(list, domain.Source{ID: int64(i + 1), URL: u})
	}
	repo := &fakeRepo{}
	uc := NewFetchNews(&slowFetcher{delay: delay}, &fakeSources{list: list}, repo)

	start := time.Now()
	if _, err := uc.Run(context.Background()); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	took := time.Since(start)

	// Последовательно 5 лент заняли бы 5 × 200ms = 1s. Параллельно — около 200ms.
	if took > 3*delay {
		t.Errorf("ленты скачивались не параллельно: заняло %s", took)
	}
	if len(repo.saved) != 5 {
		t.Errorf("ожидали 5 сохранённых новостей, получили %d", len(repo.saved))
	}
}

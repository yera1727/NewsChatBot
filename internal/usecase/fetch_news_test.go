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

type fakeSource struct {
	data map[string][]domain.Article
	errs map[string]error
}

func (f *fakeSource) Fetch(_ context.Context, url string) ([]domain.Article, error) {
	return f.data[url], f.errs[url]
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
	source := &fakeSource{
		data: map[string][]domain.Article{
			"good": {{Link: "l1"}, {Link: "l2"}},
		},
		errs: map[string]error{
			"bad": errors.New("сайт недоступен"),
		},
	}
	repo := &fakeRepo{}

	uc := NewFetchNews(source, repo, []string{"bad", "good"})
	added, err := uc.Run(context.Background())

	if err == nil {
		t.Error("ожидали ошибку от ленты bad, получили nil")
	}
	if added != 2 || len(repo.saved) != 2 {
		t.Errorf("ожидали 2 новые новости, получили added=%d saved=%d", added, len(repo.saved))
	}
}

func TestFetchNews_SecondRunFindsNothingNew(t *testing.T) {
	source := &fakeSource{
		data: map[string][]domain.Article{
			"feed": {{Link: "l1"}, {Link: "l2"}},
		},
	}
	uc := NewFetchNews(source, &fakeRepo{}, []string{"feed"})

	if added, _ := uc.Run(context.Background()); added != 2 {
		t.Errorf("первый запуск: ожидали 2 новые, получили %d", added)
	}
	if added, _ := uc.Run(context.Background()); added != 0 {
		t.Errorf("второй запуск: ожидали 0 новых, получили %d", added)
	}
}

// slowSource имитирует медленную сеть: каждая загрузка занимает delay.
type slowSource struct {
	delay time.Duration
}

func (s *slowSource) Fetch(_ context.Context, url string) ([]domain.Article, error) {
	time.Sleep(s.delay)
	return []domain.Article{{Link: url}}, nil
}

func TestFetchNews_FeedsFetchedInParallel(t *testing.T) {
	const delay = 200 * time.Millisecond
	repo := &fakeRepo{}
	uc := NewFetchNews(&slowSource{delay: delay}, repo, []string{"a", "b", "c", "d", "e"})

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

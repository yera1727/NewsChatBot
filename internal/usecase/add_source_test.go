package usecase

import (
	"context"
	"errors"
	"testing"

	"PetProjectClaude/internal/domain"
)

// fakeSourceWriter имитирует таблицу sources с уникальным url.
type fakeSourceWriter struct {
	byURL  map[string]domain.Source
	nextID int64
}

func (f *fakeSourceWriter) AddSource(_ context.Context, url, title string) (domain.Source, bool, error) {
	if src, ok := f.byURL[url]; ok {
		return src, false, nil
	}
	if f.byURL == nil {
		f.byURL = make(map[string]domain.Source)
	}
	f.nextID++
	src := domain.Source{ID: f.nextID, URL: url, Title: title, Active: true}
	f.byURL[url] = src
	return src, true, nil
}

const feedURL = "https://example.com/rss"

func goodFetcher() *fakeFetcher {
	return &fakeFetcher{feeds: map[string]domain.Feed{
		feedURL: {Title: "Пример", Articles: []domain.Article{{Link: "a1"}, {Link: "a2"}}},
	}}
}

func TestAddSource_NewSource_SavedAndSubscribed(t *testing.T) {
	articles, subs := &fakeRepo{}, &fakeSubRepo{}
	uc := NewAddSource(goodFetcher(), &fakeSourceWriter{}, articles, subs)

	src, created, err := uc.Run(context.Background(), 42, "  "+feedURL+"  ")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !created || src.Title != "Пример" {
		t.Errorf("ожидали новый источник «Пример», получили created=%v %+v", created, src)
	}
	if len(articles.saved) != 2 || articles.saved[0].Source.ID != src.ID {
		t.Errorf("статьи ленты должны сохраниться с привязкой к источнику, получили %+v", articles.saved)
	}
	if got := subs.subscribed[42]; len(got) != 1 || got[0] != src.ID {
		t.Errorf("автор команды должен быть подписан, получили %v", got)
	}
}

func TestAddSource_ExistingSource_OnlySubscribed(t *testing.T) {
	existing := domain.Source{ID: 5, URL: feedURL, Active: true}
	sources := &fakeSourceWriter{byURL: map[string]domain.Source{feedURL: existing}}
	articles, subs := &fakeRepo{}, &fakeSubRepo{}

	src, created, err := NewAddSource(goodFetcher(), sources, articles, subs).Run(context.Background(), 42, feedURL)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if created || src.ID != 5 {
		t.Errorf("ожидали существующий источник 5, получили created=%v id=%d", created, src.ID)
	}
	if len(articles.saved) != 0 {
		t.Errorf("статьи существующего источника сохраняет fetcher, а не /add: %d", len(articles.saved))
	}
	if len(subs.subscribed[42]) != 1 {
		t.Errorf("пользователь должен быть подписан")
	}
}

func TestAddSource_Errors(t *testing.T) {
	disabled := &fakeSourceWriter{byURL: map[string]domain.Source{feedURL: {ID: 1, URL: feedURL, Active: false}}}

	// Табличный тест: каждый случай — имя, входные данные и ожидаемая ошибка.
	tests := []struct {
		name    string
		url     string
		fetcher *fakeFetcher
		sources *fakeSourceWriter
		wantErr error
	}{
		{"не ссылка", "привет", goodFetcher(), &fakeSourceWriter{}, ErrInvalidURL},
		{"не http", "ftp://example.com/rss", goodFetcher(), &fakeSourceWriter{}, ErrInvalidURL},
		{"без хоста", "https://", goodFetcher(), &fakeSourceWriter{}, ErrInvalidURL},
		{"не скачалась", feedURL,
			&fakeFetcher{errs: map[string]error{feedURL: errors.New("404")}}, &fakeSourceWriter{}, ErrNotAFeed},
		{"пустая лента", feedURL,
			&fakeFetcher{feeds: map[string]domain.Feed{feedURL: {Title: "Пусто"}}}, &fakeSourceWriter{}, ErrNotAFeed},
		{"отключён", feedURL, goodFetcher(), disabled, ErrSourceDisabled},
	}

	for _, tt := range tests {
		// t.Run запускает подтест: в выводе будет видно, какой именно случай упал.
		t.Run(tt.name, func(t *testing.T) {
			subs := &fakeSubRepo{}
			_, _, err := NewAddSource(tt.fetcher, tt.sources, &fakeRepo{}, subs).Run(context.Background(), 42, tt.url)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ожидали %v, получили %v", tt.wantErr, err)
			}
			if len(subs.subscribed) != 0 {
				t.Errorf("при ошибке подписки быть не должно")
			}
		})
	}
}

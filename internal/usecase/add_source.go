package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"PetProjectClaude/internal/domain"
)

// SourceWriter — добавление источников.
type SourceWriter interface {
	// AddSource добавляет источник. Если источник с таким URL уже есть,
	// возвращает существующий и created = false.
	AddSource(ctx context.Context, url, title string) (src domain.Source, created bool, err error)
}

var (
	ErrInvalidURL     = errors.New("некорректная ссылка")
	ErrNotAFeed       = errors.New("по ссылке нет RSS-ленты")
	ErrSourceDisabled = errors.New("источник отключён")
)

// AddSource — сценарий «пользователь добавляет свою RSS-ленту».
type AddSource struct {
	fetcher  FeedFetcher
	sources  SourceWriter
	articles ArticleRepository
	subs     SubscriptionRepository
}

func NewAddSource(fetcher FeedFetcher, sources SourceWriter, articles ArticleRepository, subs SubscriptionRepository) *AddSource {
	return &AddSource{fetcher: fetcher, sources: sources, articles: articles, subs: subs}
}

// Run проверяет ленту, добавляет её в источники и подписывает на неё чат.
// Возвращает источник и признак, был ли он создан только что.
func (uc *AddSource) Run(ctx context.Context, chatID int64, rawURL string) (domain.Source, bool, error) {
	feedURL, err := normalizeFeedURL(rawURL)
	if err != nil {
		return domain.Source{}, false, err
	}

	// Скачиваем ленту ДО добавления: в базу попадут только рабочие RSS-ленты.
	feed, err := uc.fetcher.Fetch(ctx, feedURL)
	if err != nil {
		// %w дважды: снаружи можно проверить и ErrNotAFeed, и исходную причину.
		return domain.Source{}, false, fmt.Errorf("%w: %w", ErrNotAFeed, err)
	}
	if len(feed.Articles) == 0 {
		return domain.Source{}, false, fmt.Errorf("%w: лента пустая", ErrNotAFeed)
	}

	src, created, err := uc.sources.AddSource(ctx, feedURL, feed.Title)
	if err != nil {
		return domain.Source{}, false, err
	}
	if !src.Active {
		return src, false, ErrSourceDisabled
	}

	// Новый источник сразу наполняем статьями, чтобы он не был пустым до следующего
	// цикла fetcher'а. Существующий источник fetcher и так обновляет.
	if created {
		for i := range feed.Articles {
			feed.Articles[i].Source = src
		}
		if _, err := uc.articles.Save(ctx, feed.Articles); err != nil {
			return src, created, fmt.Errorf("сохранение статей нового источника: %w", err)
		}
	}

	// Подписка после сохранения статей: подписчик получит только то, что выйдет позже.
	if err := uc.subs.Subscribe(ctx, chatID, src.ID); err != nil {
		return src, created, err
	}
	return src, created, nil
}

// normalizeFeedURL проверяет, что строка — абсолютная http(s)-ссылка.
func normalizeFeedURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", ErrInvalidURL
	}
	u.Fragment = "" // часть после # сервер всё равно не получает
	return u.String(), nil
}

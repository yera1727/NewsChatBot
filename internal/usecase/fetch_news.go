// Package usecase — бизнес-логика (сценарии) приложения.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"PetProjectClaude/internal/domain"
)

// Интерфейсы объявлены здесь, у потребителя, а не в адаптерах — так принято в Go.
// Use case говорит: «мне нужно что-то, что умеет X», а кто это реализует, ему неважно.

// FeedFetcher — скачивает ленту по адресу (сейчас RSS, но может быть что угодно).
// Реализация должна быть безопасна для вызова из нескольких горутин одновременно.
type FeedFetcher interface {
	Fetch(ctx context.Context, feedURL string) (domain.Feed, error)
}

// SourceRepository — хранилище источников со стороны fetcher'а.
type SourceRepository interface {
	ActiveSources(ctx context.Context) ([]domain.Source, error)
	UpdateSourceTitle(ctx context.Context, sourceID int64, title string) error
}

// ArticleRepository — где хранить новости (сейчас PostgreSQL).
type ArticleRepository interface {
	// Save сохраняет статьи, пропуская уже известные, и возвращает число новых.
	// У каждой статьи должен быть заполнен Source.ID.
	Save(ctx context.Context, articles []domain.Article) (int, error)
}

// FetchNews — сценарий «собрать новости со всех активных источников и сохранить».
type FetchNews struct {
	fetcher  FeedFetcher
	sources  SourceRepository
	articles ArticleRepository
}

// NewFetchNews — конструктор. Зависимости передаются снаружи (dependency injection).
func NewFetchNews(fetcher FeedFetcher, sources SourceRepository, articles ArticleRepository) *FetchNews {
	return &FetchNews{fetcher: fetcher, sources: sources, articles: articles}
}

// fetchResult — результат скачивания одной ленты, который горутина отправляет в канал.
type fetchResult struct {
	source domain.Source
	feed   domain.Feed
	err    error
}

// Run скачивает все активные источники параллельно, сохраняет результаты
// и возвращает число новых статей.
// Ошибка одной ленты не останавливает остальные: ошибки копятся и возвращаются вместе.
//
// Схема:
//
//	горутина 1 (лента A) ─┐
//	горутина 2 (лента B) ─┼─→ канал results ─→ Run сохраняет по одному
//	горутина 3 (лента C) ─┘
//
// Скачивание идёт параллельно (это долго — ждём сеть), а сохранение — в одной
// горутине, поэтому репозиторию не нужно заботиться о параллельном доступе.
func (uc *FetchNews) Run(ctx context.Context) (int, error) {
	sources, err := uc.sources.ActiveSources(ctx)
	if err != nil {
		return 0, fmt.Errorf("список источников: %w", err)
	}

	// Буферизованный канал: горутины смогут положить результат, не дожидаясь читателя.
	results := make(chan fetchResult, len(sources))

	var wg sync.WaitGroup
	for _, src := range sources {
		// wg.Go запускает функцию в новой горутине и учитывает её в WaitGroup.
		wg.Go(func() {
			feed, err := uc.fetcher.Fetch(ctx, src.URL)
			results <- fetchResult{source: src, feed: feed, err: err}
		})
	}

	// Отдельная горутина ждёт завершения всех загрузок и закрывает канал.
	// Закрытый канал — сигнал для цикла `for range` ниже, что данных больше не будет.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Обрабатываем результаты по мере поступления — кто первый скачался, того и сохраняем.
	var (
		errs     []error
		newTotal int
	)
	for r := range results {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		added, err := uc.save(ctx, r.source, r.feed)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		newTotal += added
	}

	return newTotal, errors.Join(errs...) // errors.Join вернёт nil, если ошибок не было
}

// save обновляет название источника (если в ленте оно поменялось) и сохраняет статьи.
func (uc *FetchNews) save(ctx context.Context, src domain.Source, feed domain.Feed) (int, error) {
	if feed.Title != "" && feed.Title != src.Title {
		if err := uc.sources.UpdateSourceTitle(ctx, src.ID, feed.Title); err != nil {
			return 0, fmt.Errorf("название источника %s: %w", src.URL, err)
		}
		src.Title = feed.Title
	}

	// Адаптер RSS не знает про наши источники — привязываем статьи к источнику здесь.
	for i := range feed.Articles {
		feed.Articles[i].Source = src
	}

	added, err := uc.articles.Save(ctx, feed.Articles)
	if err != nil {
		return 0, fmt.Errorf("сохранение %s: %w", src.URL, err)
	}
	return added, nil
}

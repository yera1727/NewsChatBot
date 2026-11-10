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

// FeedSource — откуда брать новости (сейчас RSS, но может быть что угодно).
// Реализация должна быть безопасна для вызова из нескольких горутин одновременно.
type FeedSource interface {
	Fetch(ctx context.Context, feedURL string) ([]domain.Article, error)
}

// ArticleRepository — где хранить новости (сейчас PostgreSQL).
type ArticleRepository interface {
	// Save сохраняет статьи, пропуская уже известные, и возвращает число новых.
	Save(ctx context.Context, articles []domain.Article) (int, error)
}

// FetchNews — сценарий «собрать новости со всех лент и сохранить».
type FetchNews struct {
	source FeedSource
	repo   ArticleRepository
	feeds  []string
}

// NewFetchNews — конструктор. Зависимости передаются снаружи (dependency injection).
func NewFetchNews(source FeedSource, repo ArticleRepository, feeds []string) *FetchNews {
	return &FetchNews{source: source, repo: repo, feeds: feeds}
}

// fetchResult — результат скачивания одной ленты, который горутина отправляет в канал.
type fetchResult struct {
	feed     string
	articles []domain.Article
	err      error
}

// Run скачивает все ленты параллельно, сохраняет результаты и возвращает число новых статей.
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
	// Буферизованный канал: горутины смогут положить результат, не дожидаясь читателя.
	results := make(chan fetchResult, len(uc.feeds))

	var wg sync.WaitGroup
	for _, feed := range uc.feeds {
		// wg.Go запускает функцию в новой горутине и учитывает её в WaitGroup.
		wg.Go(func() {
			articles, err := uc.source.Fetch(ctx, feed)
			results <- fetchResult{feed: feed, articles: articles, err: err}
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
		added, err := uc.repo.Save(ctx, r.articles)
		if err != nil {
			errs = append(errs, fmt.Errorf("сохранение %s: %w", r.feed, err))
			continue
		}
		newTotal += added
	}

	return newTotal, errors.Join(errs...) // errors.Join вернёт nil, если ошибок не было
}

// Package feedrouter выбирает, каким адаптером скачивать ленту, по её адресу:
// ссылки на Telegram — адаптером tgchannel, всё остальное — RSS.
//
// Сам Router тоже реализует usecase.FeedFetcher, поэтому use case'ы не знают,
// что адаптеров несколько. Это паттерн «компоновщик» (composite): добавить
// новый тип источника (например, YouTube) — значит добавить адаптер и строчку здесь.
package feedrouter

import (
	"context"

	"PetProjectClaude/internal/adapter/tgchannel"
	"PetProjectClaude/internal/domain"
)

// fetcher — то же, что usecase.FeedFetcher. Объявляем локально: пакету нужен
// только метод Fetch, и так он не зависит от пакета usecase.
type fetcher interface {
	Fetch(ctx context.Context, feedURL string) (domain.Feed, error)
}

type Router struct {
	rss      fetcher
	telegram fetcher
}

func New(rss, telegram fetcher) *Router {
	return &Router{rss: rss, telegram: telegram}
}

func (r *Router) Fetch(ctx context.Context, feedURL string) (domain.Feed, error) {
	if tgchannel.IsTelegramURL(feedURL) {
		return r.telegram.Fetch(ctx, feedURL)
	}
	return r.rss.Fetch(ctx, feedURL)
}

package usecase

import (
	"context"

	"PetProjectClaude/internal/domain"
)

// LatestNews — сценарий «показать последние новости».
type LatestNews struct {
	articles ArticleReader
}

func NewLatestNews(articles ArticleReader) *LatestNews {
	return &LatestNews{articles: articles}
}

func (uc *LatestNews) Run(ctx context.Context, limit int) ([]domain.Article, error) {
	return uc.articles.Latest(ctx, limit)
}

// Subscriptions — сценарии управления подписками.
// Пользователь выбирает источник по номеру из списка Sources (нумерация с 1).
type Subscriptions struct {
	articles ArticleReader
	repo     SubscriptionRepository
}

func NewSubscriptions(articles ArticleReader, repo SubscriptionRepository) *Subscriptions {
	return &Subscriptions{articles: articles, repo: repo}
}

func (uc *Subscriptions) Sources(ctx context.Context) ([]string, error) {
	return uc.articles.Sources(ctx)
}

// Subscribe подписывает чат на источник с номером number и возвращает этот источник.
func (uc *Subscriptions) Subscribe(ctx context.Context, chatID int64, number int) (string, error) {
	source, err := uc.sourceByNumber(ctx, number)
	if err != nil {
		return "", err
	}
	return source, uc.repo.Subscribe(ctx, chatID, source)
}

// Unsubscribe отписывает чат от источника с номером number и возвращает этот источник.
func (uc *Subscriptions) Unsubscribe(ctx context.Context, chatID int64, number int) (string, error) {
	source, err := uc.sourceByNumber(ctx, number)
	if err != nil {
		return "", err
	}
	return source, uc.repo.Unsubscribe(ctx, chatID, source)
}

func (uc *Subscriptions) List(ctx context.Context, chatID int64) ([]string, error) {
	return uc.repo.ListByChat(ctx, chatID)
}

func (uc *Subscriptions) sourceByNumber(ctx context.Context, number int) (string, error) {
	sources, err := uc.articles.Sources(ctx)
	if err != nil {
		return "", err
	}
	if number < 1 || number > len(sources) {
		return "", ErrUnknownSource
	}
	return sources[number-1], nil
}

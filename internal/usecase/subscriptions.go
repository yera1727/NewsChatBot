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
// Пользователь выбирает источник по его ID — он не меняется при добавлении новых источников.
type Subscriptions struct {
	sources SourceReader
	repo    SubscriptionRepository
}

func NewSubscriptions(sources SourceReader, repo SubscriptionRepository) *Subscriptions {
	return &Subscriptions{sources: sources, repo: repo}
}

// Sources возвращает источники, на которые можно подписаться.
func (uc *Subscriptions) Sources(ctx context.Context) ([]domain.Source, error) {
	return uc.sources.Sources(ctx)
}

// Subscribe подписывает чат на источник. На неактивный источник подписаться нельзя.
func (uc *Subscriptions) Subscribe(ctx context.Context, chatID, sourceID int64) (domain.Source, error) {
	src, err := uc.sources.SourceByID(ctx, sourceID)
	if err != nil {
		return domain.Source{}, err
	}
	if !src.Active {
		return domain.Source{}, ErrUnknownSource
	}
	return src, uc.repo.Subscribe(ctx, chatID, src.ID)
}

// Unsubscribe отписывает чат от источника. Отписаться можно и от неактивного.
func (uc *Subscriptions) Unsubscribe(ctx context.Context, chatID, sourceID int64) (domain.Source, error) {
	src, err := uc.sources.SourceByID(ctx, sourceID)
	if err != nil {
		return domain.Source{}, err
	}
	return src, uc.repo.Unsubscribe(ctx, chatID, src.ID)
}

func (uc *Subscriptions) List(ctx context.Context, chatID int64) ([]domain.Source, error) {
	return uc.repo.ListByChat(ctx, chatID)
}

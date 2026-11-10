package usecase

import (
	"context"
	"errors"

	"PetProjectClaude/internal/domain"
)

// Интерфейсы («порты»), которые нужны сценариям бота.
// Реализуют их адаптеры: postgres (хранилище) и telegram (отправка сообщений).

// ArticleReader — чтение сохранённых новостей.
type ArticleReader interface {
	// LatestForChat возвращает самые свежие статьи из источников, на которые подписан чат.
	LatestForChat(ctx context.Context, chatID int64, limit int) ([]domain.Article, error)
}

// SourceReader — чтение источников.
type SourceReader interface {
	// Sources возвращает активные источники, упорядоченные по ID.
	Sources(ctx context.Context) ([]domain.Source, error)
	// SourceByID возвращает источник (в том числе неактивный) или ErrUnknownSource.
	SourceByID(ctx context.Context, id int64) (domain.Source, error)
}

// SubscriptionRepository — хранение подписок и прогресса доставки.
type SubscriptionRepository interface {
	// Subscribe подписывает чат на источник. Старые статьи считаются уже доставленными,
	// чтобы новый подписчик не получил сотни сообщений разом.
	Subscribe(ctx context.Context, chatID, sourceID int64) error
	Unsubscribe(ctx context.Context, chatID, sourceID int64) error
	UnsubscribeAll(ctx context.Context, chatID int64) error
	ListByChat(ctx context.Context, chatID int64) ([]domain.Source, error)

	// Pending возвращает ещё не доставленные новости по всем подпискам, старые первыми.
	Pending(ctx context.Context, limit int) ([]domain.Delivery, error)
	// MarkDelivered запоминает, что статья articleID (и все до неё) доставлена.
	MarkDelivered(ctx context.Context, chatID, sourceID, articleID int64) error
}

// Notifier — отправка новости пользователю (сейчас Telegram).
type Notifier interface {
	Notify(ctx context.Context, chatID int64, article domain.Article) error
}

// Ошибки бизнес-логики. Адаптеры переводят в них свои ошибки (например, ответ 403
// от Telegram или «строка не найдена» от БД), а use case'ы проверяют их через
// errors.Is — не зная ни о Telegram, ни о PostgreSQL.
var (
	ErrUnknownSource   = errors.New("неизвестный источник")
	ErrNoSubscriptions = errors.New("нет подписок")
	ErrChatUnavailable = errors.New("чат недоступен") // пользователь заблокировал бота
)

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
	Latest(ctx context.Context, limit int) ([]domain.Article, error)
	// Sources возвращает все известные источники в стабильном порядке.
	Sources(ctx context.Context) ([]string, error)
}

// SubscriptionRepository — хранение подписок и прогресса доставки.
type SubscriptionRepository interface {
	// Subscribe подписывает чат на источник. Старые статьи считаются уже доставленными,
	// чтобы новый подписчик не получил сотни сообщений разом.
	Subscribe(ctx context.Context, chatID int64, source string) error
	Unsubscribe(ctx context.Context, chatID int64, source string) error
	UnsubscribeAll(ctx context.Context, chatID int64) error
	ListByChat(ctx context.Context, chatID int64) ([]string, error)

	// Pending возвращает ещё не доставленные новости по всем подпискам, старые первыми.
	Pending(ctx context.Context, limit int) ([]domain.Delivery, error)
	// MarkDelivered запоминает, что статья articleID (и все до неё) доставлена.
	MarkDelivered(ctx context.Context, chatID int64, source string, articleID int64) error
}

// Notifier — отправка новости пользователю (сейчас Telegram).
type Notifier interface {
	Notify(ctx context.Context, chatID int64, article domain.Article) error
}

// Ошибки бизнес-логики. Адаптеры переводят в них свои ошибки (например, ответ 403
// от Telegram), а use case'ы проверяют их через errors.Is — не зная о Telegram.
var (
	ErrUnknownSource   = errors.New("неизвестный источник")
	ErrChatUnavailable = errors.New("чат недоступен") // пользователь заблокировал бота
)

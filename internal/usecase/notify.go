package usecase

import (
	"context"
	"errors"
	"fmt"
)

// NotifySubscribers — сценарий «разослать подписчикам новые статьи».
type NotifySubscribers struct {
	repo      SubscriptionRepository
	notifier  Notifier
	batchSize int // сколько сообщений максимум отправить за один запуск
}

func NewNotifySubscribers(repo SubscriptionRepository, notifier Notifier, batchSize int) *NotifySubscribers {
	return &NotifySubscribers{repo: repo, notifier: notifier, batchSize: batchSize}
}

// Run отправляет накопившиеся новости и возвращает, сколько сообщений доставлено.
//
// Статья отмечается доставленной только ПОСЛЕ успешной отправки. Если отправка
// упала, статья останется в очереди и уйдёт при следующем запуске.
func (uc *NotifySubscribers) Run(ctx context.Context) (int, error) {
	deliveries, err := uc.repo.Pending(ctx, uc.batchSize)
	if err != nil {
		return 0, fmt.Errorf("получение очереди рассылки: %w", err)
	}

	var (
		sent int
		errs []error
		// Если отправка в чат не удалась, остальные его статьи в этот раз пропускаем,
		// чтобы не нарушить порядок (иначе статья 6 «перепрыгнет» неотправленную 5).
		skip = make(map[int64]bool)
	)

	for _, d := range deliveries {
		if skip[d.ChatID] {
			continue
		}

		err := uc.notifier.Notify(ctx, d.ChatID, d.Article)
		switch {
		case errors.Is(err, ErrChatUnavailable):
			// Пользователь заблокировал бота — писать ему бессмысленно, удаляем подписки.
			// Иначе его очередь росла бы вечно и забивала рассылку остальным.
			skip[d.ChatID] = true
			if err := uc.repo.UnsubscribeAll(ctx, d.ChatID); err != nil {
				errs = append(errs, fmt.Errorf("отписка недоступного чата %d: %w", d.ChatID, err))
			}
			continue
		case err != nil:
			skip[d.ChatID] = true
			errs = append(errs, fmt.Errorf("отправка в чат %d: %w", d.ChatID, err))
			continue
		}

		if err := uc.repo.MarkDelivered(ctx, d.ChatID, d.Article.Source, d.Article.ID); err != nil {
			// Не смогли записать прогресс — останавливаемся, иначе разошлём дубли.
			return sent, errors.Join(append(errs, fmt.Errorf("отметка доставки: %w", err))...)
		}
		sent++
	}

	return sent, errors.Join(errs...)
}

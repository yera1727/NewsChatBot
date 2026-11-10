package postgres

import (
	"context"
	"fmt"

	"PetProjectClaude/internal/domain"
)

// Subscribe подписывает чат на источник. last_article_id сразу ставим на самую
// свежую статью источника, чтобы подписчику пришли только новые, а не весь архив.
// ON CONFLICT DO NOTHING — повторная подписка ничего не ломает.
func (r *Repository) Subscribe(ctx context.Context, chatID, sourceID int64) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO subscriptions (chat_id, source_id, last_article_id)
		VALUES ($1, $2, (SELECT coalesce(max(id), 0) FROM articles WHERE source_id = $2))
		ON CONFLICT (chat_id, source_id) DO NOTHING`, chatID, sourceID)
	if err != nil {
		return fmt.Errorf("подписка: %w", err)
	}
	return nil
}

func (r *Repository) Unsubscribe(ctx context.Context, chatID, sourceID int64) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM subscriptions WHERE chat_id = $1 AND source_id = $2`, chatID, sourceID)
	if err != nil {
		return fmt.Errorf("отписка: %w", err)
	}
	return nil
}

func (r *Repository) UnsubscribeAll(ctx context.Context, chatID int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM subscriptions WHERE chat_id = $1`, chatID)
	if err != nil {
		return fmt.Errorf("отписка от всего: %w", err)
	}
	return nil
}

func (r *Repository) ListByChat(ctx context.Context, chatID int64) ([]domain.Source, error) {
	return r.querySources(ctx, `
		SELECT s.id, s.url, s.title, s.active
		FROM subscriptions sub
		JOIN sources s ON s.id = sub.source_id
		WHERE sub.chat_id = $1
		ORDER BY s.id`, chatID)
}

// Pending находит статьи, которые подписчики ещё не получили:
// для каждой подписки — статьи её источника с id больше last_article_id.
func (r *Repository) Pending(ctx context.Context, limit int) ([]domain.Delivery, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+articleColumns+`, sub.chat_id
		FROM subscriptions sub
		JOIN articles a ON a.source_id = sub.source_id AND a.id > sub.last_article_id
		JOIN sources s  ON s.id = a.source_id
		ORDER BY a.id
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("очередь рассылки: %w", err)
	}
	defer rows.Close()

	var deliveries []domain.Delivery
	for rows.Next() {
		var d domain.Delivery
		if d.Article, err = scanArticle(rows, &d.ChatID); err != nil {
			return nil, err
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

// MarkDelivered сдвигает прогресс подписки. Условие last_article_id < $3
// защищает от отката назад, если вызовы придут не по порядку.
func (r *Repository) MarkDelivered(ctx context.Context, chatID, sourceID, articleID int64) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE subscriptions SET last_article_id = $3
		WHERE chat_id = $1 AND source_id = $2 AND last_article_id < $3`, chatID, sourceID, articleID)
	if err != nil {
		return fmt.Errorf("отметка доставки: %w", err)
	}
	return nil
}

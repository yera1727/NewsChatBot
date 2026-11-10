package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"PetProjectClaude/internal/domain"
)

// Subscribe подписывает чат на источник. last_article_id сразу ставим на самую
// свежую статью источника, чтобы подписчику пришли только новые, а не весь архив.
// ON CONFLICT DO NOTHING — повторная подписка ничего не ломает.
func (r *Repository) Subscribe(ctx context.Context, chatID int64, source string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO subscriptions (chat_id, source, last_article_id)
		VALUES ($1, $2, (SELECT coalesce(max(id), 0) FROM articles WHERE source = $2))
		ON CONFLICT (chat_id, source) DO NOTHING`, chatID, source)
	if err != nil {
		return fmt.Errorf("подписка: %w", err)
	}
	return nil
}

func (r *Repository) Unsubscribe(ctx context.Context, chatID int64, source string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM subscriptions WHERE chat_id = $1 AND source = $2`, chatID, source)
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

func (r *Repository) ListByChat(ctx context.Context, chatID int64) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT source FROM subscriptions WHERE chat_id = $1 ORDER BY source`, chatID)
	if err != nil {
		return nil, fmt.Errorf("подписки чата: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// Pending находит статьи, которые подписчики ещё не получили:
// для каждой подписки — статьи её источника с id больше last_article_id.
func (r *Repository) Pending(ctx context.Context, limit int) ([]domain.Delivery, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT a.id, a.source, a.title, a.link, a.description, a.published_at, s.chat_id
		FROM subscriptions s
		JOIN articles a ON a.source = s.source AND a.id > s.last_article_id
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
func (r *Repository) MarkDelivered(ctx context.Context, chatID int64, source string, articleID int64) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE subscriptions SET last_article_id = $3
		WHERE chat_id = $1 AND source = $2 AND last_article_id < $3`, chatID, source, articleID)
	if err != nil {
		return fmt.Errorf("отметка доставки: %w", err)
	}
	return nil
}

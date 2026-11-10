// Package postgres — адаптер: хранит новости в PostgreSQL.
// Реализует интерфейсы usecase.ArticleRepository, usecase.ArticleReader
// и usecase.SubscriptionRepository (последний — в файле subscriptions.go).
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"PetProjectClaude/internal/domain"
)

type Repository struct {
	pool *pgxpool.Pool // пул соединений: переиспользует подключения к БД
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Вставляем все статьи одним запросом. unnest превращает массивы-параметры в строки таблицы:
//
//	$1 = {src1, src2}, $2 = {title1, title2}, ...  →  (src1, title1, ...), (src2, title2, ...)
//
// ON CONFLICT (link) DO NOTHING — если статья с такой ссылкой уже есть, тихо пропускаем её.
const insertArticles = `
INSERT INTO articles (source, title, link, description, published_at)
SELECT * FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::timestamptz[])
ON CONFLICT (link) DO NOTHING`

// Save сохраняет статьи и возвращает, сколько из них оказались новыми.
func (r *Repository) Save(ctx context.Context, articles []domain.Article) (int, error) {
	if len(articles) == 0 {
		return 0, nil
	}

	n := len(articles)
	sources := make([]string, 0, n)
	titles := make([]string, 0, n)
	links := make([]string, 0, n)
	descriptions := make([]string, 0, n)
	published := make([]*time.Time, 0, n) // указатель, чтобы передать NULL вместо «нулевой» даты

	for _, a := range articles {
		sources = append(sources, a.Source)
		titles = append(titles, a.Title)
		links = append(links, a.Link)
		descriptions = append(descriptions, a.Description)

		if a.PublishedAt.IsZero() {
			published = append(published, nil)
		} else {
			t := a.PublishedAt
			published = append(published, &t)
		}
	}

	tag, err := r.pool.Exec(ctx, insertArticles, sources, titles, links, descriptions, published)
	if err != nil {
		return 0, fmt.Errorf("вставка статей: %w", err)
	}
	// RowsAffected считает только реально вставленные строки (без пропущенных дублей).
	return int(tag.RowsAffected()), nil
}

// Latest возвращает самые свежие статьи.
func (r *Repository) Latest(ctx context.Context, limit int) ([]domain.Article, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, source, title, link, description, published_at
		FROM articles
		ORDER BY published_at DESC NULLS LAST
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("последние статьи: %w", err)
	}
	defer rows.Close()

	var articles []domain.Article
	for rows.Next() {
		a, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err() // rows.Err() — ошибка, случившаяся во время чтения строк
}

// Sources возвращает все источники, отсортированные по алфавиту.
// Порядок важен: пользователь выбирает источник по номеру в этом списке.
func (r *Repository) Sources(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT source FROM articles ORDER BY source`)
	if err != nil {
		return nil, fmt.Errorf("список источников: %w", err)
	}
	// pgx.CollectRows читает все строки; RowTo[string] — каждую строку в string.
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// scanArticle читает статью из строки результата. Первыми колонками у
// вызывающего запроса должны идти: id, source, title, link, description, published_at.
// pgx.Row — общий интерфейс и для одной строки, и для курсора rows.
func scanArticle(row pgx.Row, extra ...any) (domain.Article, error) {
	var (
		a         domain.Article
		published *time.Time // в БД может быть NULL
	)
	dest := append([]any{&a.ID, &a.Source, &a.Title, &a.Link, &a.Description, &published}, extra...)
	if err := row.Scan(dest...); err != nil {
		return domain.Article{}, fmt.Errorf("чтение статьи: %w", err)
	}
	if published != nil {
		a.PublishedAt = *published
	}
	return a, nil
}

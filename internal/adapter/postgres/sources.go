package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"PetProjectClaude/internal/domain"
	"PetProjectClaude/internal/usecase"
)

const sourceColumns = `id, url, title, active`

// EnsureSources добавляет источники, которых ещё нет в БД (уже существующие не трогает).
// Используется при старте fetcher'а, чтобы засеять базу лентами из конфига.
//
// Почему WHERE NOT EXISTS, а не просто ON CONFLICT DO NOTHING: в PostgreSQL каждая
// попытка INSERT берёт следующий номер из счётчика id, даже если строка в итоге
// не вставилась. Без проверки каждый перезапуск fetcher'а «съедал» бы номера,
// и пользователи видели бы источники 1, 2, 5, 8... ON CONFLICT остаётся страховкой
// на случай, если два процесса вставляют одновременно.
func (r *Repository) EnsureSources(ctx context.Context, urls []string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO sources (url)
		SELECT u FROM unnest($1::text[]) AS u
		WHERE NOT EXISTS (SELECT 1 FROM sources WHERE url = u)
		ON CONFLICT (url) DO NOTHING`, urls)
	if err != nil {
		return fmt.Errorf("добавление источников: %w", err)
	}
	return nil
}

// AddSource добавляет источник или возвращает существующий с тем же URL.
func (r *Repository) AddSource(ctx context.Context, url, title string) (domain.Source, bool, error) {
	// Сначала ищем: так повторный /add не тратит номер из счётчика (см. EnsureSources).
	if src, err := r.sourceByURL(ctx, url); err == nil {
		return src, false, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Source{}, false, fmt.Errorf("поиск источника: %w", err)
	}

	// ON CONFLICT DO NOTHING + RETURNING: если строка вставилась — получим её.
	// Если между поиском и вставкой источник успел добавить кто-то другой,
	// запрос не вернёт строк (pgx.ErrNoRows) — тогда просто читаем существующий.
	row := r.pool.QueryRow(ctx, `
		INSERT INTO sources (url, title) VALUES ($1, $2)
		ON CONFLICT (url) DO NOTHING
		RETURNING `+sourceColumns, url, title)

	src, err := scanSource(row)
	switch {
	case err == nil:
		return src, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		src, err = r.sourceByURL(ctx, url)
		if err != nil {
			return domain.Source{}, false, fmt.Errorf("чтение источника: %w", err)
		}
		return src, false, nil
	default:
		return domain.Source{}, false, fmt.Errorf("добавление источника: %w", err)
	}
}

func (r *Repository) sourceByURL(ctx context.Context, url string) (domain.Source, error) {
	return scanSource(r.pool.QueryRow(ctx, `SELECT `+sourceColumns+` FROM sources WHERE url = $1`, url))
}

// ActiveSources — источники, которые fetcher должен опрашивать.
func (r *Repository) ActiveSources(ctx context.Context) ([]domain.Source, error) {
	return r.querySources(ctx, `SELECT `+sourceColumns+` FROM sources WHERE active ORDER BY id`)
}

// Sources — источники, на которые можно подписаться. Порядок по ID стабилен:
// новые источники добавляются в конец, номера старых не меняются.
func (r *Repository) Sources(ctx context.Context) ([]domain.Source, error) {
	return r.ActiveSources(ctx)
}

// SourceByID возвращает источник или usecase.ErrUnknownSource, если его нет.
func (r *Repository) SourceByID(ctx context.Context, id int64) (domain.Source, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+sourceColumns+` FROM sources WHERE id = $1`, id)

	src, err := scanSource(row)
	if errors.Is(err, pgx.ErrNoRows) {
		// Переводим ошибку БД в ошибку бизнес-логики: use case не должен знать про pgx.
		return domain.Source{}, usecase.ErrUnknownSource
	}
	if err != nil {
		return domain.Source{}, fmt.Errorf("источник %d: %w", id, err)
	}
	return src, nil
}

func (r *Repository) UpdateSourceTitle(ctx context.Context, sourceID int64, title string) error {
	_, err := r.pool.Exec(ctx, `UPDATE sources SET title = $2 WHERE id = $1`, sourceID, title)
	if err != nil {
		return fmt.Errorf("обновление названия источника: %w", err)
	}
	return nil
}

func (r *Repository) querySources(ctx context.Context, sql string, args ...any) ([]domain.Source, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("список источников: %w", err)
	}
	// CollectRows читает все строки, вызывая для каждой нашу функцию scanSource.
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Source, error) {
		return scanSource(row)
	})
}

func scanSource(row pgx.Row) (domain.Source, error) {
	var s domain.Source
	err := row.Scan(&s.ID, &s.URL, &s.Title, &s.Active)
	return s, err
}

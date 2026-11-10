package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect создаёт пул соединений и проверяет, что БД отвечает.
// pgxpool.New сам не подключается, поэтому делаем Ping — так ошибка
// всплывёт сразу при старте сервиса, а не на первом запросе.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("неверная строка подключения: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("БД недоступна: %w", err)
	}
	return pool, nil
}

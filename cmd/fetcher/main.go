// fetcher — сервис, который собирает новости из RSS-лент.
// main — composition root: здесь создаются все зависимости и связываются друг с другом.
// Это единственное место, которое знает о конкретных реализациях.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"PetProjectClaude/internal/adapter/postgres"
	"PetProjectClaude/internal/adapter/rss"
	"PetProjectClaude/internal/config"
	"PetProjectClaude/internal/usecase"
)

func main() {
	// slog — структурированный логгер из стандартной библиотеки:
	// пишет пары ключ=значение, которые удобно искать и фильтровать.
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("ошибка конфигурации", "err", err)
		os.Exit(1)
	}

	// Контекст отменится, когда придёт Ctrl+C (os.Interrupt) или SIGTERM
	// (его шлёт Docker/Kubernetes при остановке контейнера).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Подключение к PostgreSQL (пул соединений).
	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("подключение к БД (запущен ли docker compose up -d?)", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Адаптеры (внешний слой).
	fetcher := rss.NewClient(cfg.RequestTimeout)
	repo := postgres.NewRepository(pool)

	// Начальное заполнение (seed): ленты из конфига добавляются в таблицу sources,
	// если их там ещё нет. Дальше fetcher работает со списком из БД.
	if err := repo.EnsureSources(ctx, cfg.Feeds); err != nil {
		logger.Error("добавление источников из конфига", "err", err)
		os.Exit(1)
	}

	// Use case получает адаптеры через интерфейсы. repo реализует сразу два из них.
	fetchNews := usecase.NewFetchNews(fetcher, repo, repo)

	logger.Info("fetcher запущен", "interval", cfg.FetchInterval)

	// Один цикл сбора. Таймаут = интервал: следующий цикл не должен начаться,
	// пока не закончился предыдущий.
	runOnce := func() {
		runCtx, cancel := context.WithTimeout(ctx, cfg.FetchInterval)
		defer cancel()

		start := time.Now()
		added, err := fetchNews.Run(runCtx)
		if err != nil {
			logger.Warn("ошибки при сборе новостей", "err", err)
		}
		logger.Info("сбор завершён", "new", added, "took", time.Since(start).Round(time.Millisecond))
	}

	runOnce() // первый сбор сразу, не дожидаясь первого тика

	ticker := time.NewTicker(cfg.FetchInterval)
	defer ticker.Stop()

	// Главный цикл: select ждёт, что случится раньше — сигнал остановки или тик таймера.
	for {
		select {
		case <-ctx.Done():
			logger.Info("получен сигнал остановки, завершаем работу")
			return
		case <-ticker.C:
			runOnce()
		}
	}
}

// bot — сервис Telegram-бота: отвечает на команды и рассылает новости подписчикам.
// Работает с той же БД, куда пишет fetcher. Сервисы не вызывают друг друга напрямую:
// fetcher кладёт статьи в базу, бот забирает их оттуда.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"PetProjectClaude/internal/adapter/feedrouter"
	"PetProjectClaude/internal/adapter/postgres"
	"PetProjectClaude/internal/adapter/rss"
	"PetProjectClaude/internal/adapter/telegram"
	"PetProjectClaude/internal/adapter/tgchannel"
	"PetProjectClaude/internal/config"
	"PetProjectClaude/internal/usecase"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("ошибка конфигурации", "err", err)
		os.Exit(1)
	}
	if cfg.TelegramToken == "" {
		logger.Error("TELEGRAM_TOKEN не задан: впишите токен бота в файл .env")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("подключение к БД (запущен ли docker compose up -d?)", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Адаптер хранилища реализует сразу несколько интерфейсов use case'ов.
	repo := postgres.NewRepository(pool)
	// Адаптеры лент нужны боту для проверки источников, которые добавляют пользователи (/add).
	fetcher := feedrouter.New(rss.NewClient(cfg.RequestTimeout), tgchannel.NewClient(cfg.RequestTimeout))

	tg, err := telegram.New(cfg.TelegramToken, telegram.UseCases{
		Latest:        usecase.NewLatestNews(repo),
		Subscriptions: usecase.NewSubscriptions(repo, repo),
		AddSource:     usecase.NewAddSource(fetcher, repo, repo, repo),
	}, logger)
	if err != nil {
		logger.Error("запуск Telegram-бота (проверьте токен)", "err", err)
		os.Exit(1)
	}

	// tg одновременно и принимает команды, и служит Notifier'ом для рассылки.
	notify := usecase.NewNotifySubscribers(repo, tg, 50)

	// Рассылка работает в отдельной горутине параллельно с приёмом команд.
	go runNotifier(ctx, notify, cfg.NotifyInterval, logger)

	logger.Info("бот запущен", "notify_interval", cfg.NotifyInterval)
	tg.Start(ctx) // блокируется, пока не придёт Ctrl+C / SIGTERM
	logger.Info("бот остановлен")
}

// runNotifier периодически запускает рассылку, пока не отменят ctx.
func runNotifier(ctx context.Context, notify *usecase.NotifySubscribers, interval time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sent, err := notify.Run(ctx)
			if err != nil {
				logger.Warn("ошибки при рассылке", "err", err)
			}
			if sent > 0 {
				logger.Info("разослано новостей", "sent", sent)
			}
		}
	}
}

// Package config читает настройки сервиса из переменных окружения.
// Переменные окружения — стандартный способ конфигурации микросервисов
// (так их удобно задавать в Docker и Kubernetes).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config — настройки обоих сервисов. Каждый сервис берёт то, что ему нужно.
type Config struct {
	DatabaseURL    string        // строка подключения к PostgreSQL
	FetchInterval  time.Duration // fetcher: как часто опрашивать ленты
	RequestTimeout time.Duration // fetcher: таймаут на скачивание одной ленты
	Feeds          []string      // fetcher: ленты для начального заполнения БД (может быть пустым)
	TelegramToken  string        // bot: токен от @BotFather
	NotifyInterval time.Duration // bot: как часто проверять, есть ли что разослать
}

// Load возвращает конфиг: значения по умолчанию, переопределённые переменными окружения.
//
//	DATABASE_URL    — по умолчанию локальная БД из docker-compose.yml
//	FETCH_INTERVAL  — например "5m", "30s" (по умолчанию 5m)
//	REQUEST_TIMEOUT — например "10s" (по умолчанию 10s)
//	FEEDS           — необязательно: ленты через запятую, которые fetcher добавит
//	                  в БД при старте. Обычно источники добавляются через /add в боте.
//	TELEGRAM_TOKEN  — токен бота
//	NOTIFY_INTERVAL — например "30s" (по умолчанию 30s)
//
// Переменные также читаются из файла .env (если он есть). Уже заданные
// в окружении переменные имеют приоритет над .env.
func Load() (Config, error) {
	loadDotEnv()

	cfg := Config{
		DatabaseURL:    "postgres://newsbot:newsbot@localhost:5433/newsbot?sslmode=disable",
		FetchInterval:  5 * time.Minute,
		RequestTimeout: 10 * time.Second,
		TelegramToken:  strings.TrimSpace(os.Getenv("TELEGRAM_TOKEN")),
		NotifyInterval: 30 * time.Second,
	}

	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}

	var err error
	if cfg.NotifyInterval, err = durationEnv("NOTIFY_INTERVAL", cfg.NotifyInterval); err != nil {
		return Config{}, err
	}
	if cfg.FetchInterval, err = durationEnv("FETCH_INTERVAL", cfg.FetchInterval); err != nil {
		return Config{}, err
	}
	if cfg.RequestTimeout, err = durationEnv("REQUEST_TIMEOUT", cfg.RequestTimeout); err != nil {
		return Config{}, err
	}

	if v := os.Getenv("FEEDS"); v != "" {
		var feeds []string
		for _, f := range strings.Split(v, ",") {
			if f = strings.TrimSpace(f); f != "" {
				feeds = append(feeds, f)
			}
		}
		if len(feeds) == 0 {
			return Config{}, fmt.Errorf("FEEDS задана, но не содержит ни одной ленты")
		}
		cfg.Feeds = feeds
	}

	return cfg, nil
}

// loadDotEnv ищет файл .env в текущей папке и выше (до 5 уровней):
// так он найдётся, даже если сервис запущен из подпапки, например cmd/bot.
func loadDotEnv() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for range 5 {
		path := filepath.Join(dir, ".env")
		if _, err := os.Stat(path); err == nil {
			_ = godotenv.Load(path) // Load не перезаписывает уже заданные переменные
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir { // дошли до корня диска
			return
		}
		dir = parent
	}
}

// durationEnv читает длительность из переменной окружения или возвращает значение по умолчанию.
func durationEnv(name string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(name)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s=%q: %w", name, v, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s должна быть больше нуля, получено %s", name, d)
	}
	return d, nil
}

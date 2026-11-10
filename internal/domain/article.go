// Package domain — ядро приложения: бизнес-сущности.
// Этот пакет ни от чего не зависит: ни от RSS, ни от БД, ни от Telegram.
package domain

import "time"

// Article — новость, с которой работает всё приложение.
type Article struct {
	ID          int64  // присваивается при сохранении в хранилище; 0 — ещё не сохранена
	Source      string // URL ленты, откуда пришла новость
	Title       string
	Link        string
	Description string
	PublishedAt time.Time
}

// Delivery — новость, которую нужно доставить подписчику.
type Delivery struct {
	ChatID  int64
	Article Article
}

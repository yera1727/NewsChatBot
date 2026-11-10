// Package domain — ядро приложения: бизнес-сущности.
// Этот пакет ни от чего не зависит: ни от RSS, ни от БД, ни от Telegram.
package domain

import (
	"net/url"
	"strings"
	"time"
)

// Source — источник новостей (RSS-лента).
type Source struct {
	ID     int64
	URL    string
	Title  string // название ленты из RSS; может быть пустым, пока fetcher её не скачал
	Active bool   // неактивные источники не опрашиваются и не предлагаются для подписки
}

// Name — человекочитаемое имя: название ленты, а если его нет — домен из URL.
func (s Source) Name() string {
	if s.Title != "" {
		return s.Title
	}
	if u, err := url.Parse(s.URL); err == nil && u.Host != "" {
		return strings.TrimPrefix(u.Host, "www.")
	}
	return s.URL
}

// Article — новость, с которой работает всё приложение.
type Article struct {
	ID          int64  // присваивается при сохранении в хранилище; 0 — ещё не сохранена
	Source      Source // откуда пришла новость
	Title       string
	Link        string
	Description string
	PublishedAt time.Time
}

// Feed — содержимое ленты, как его вернул источник.
type Feed struct {
	// URL — канонический адрес ленты, если он отличается от запрошенного
	// (например, t.me/durov → t.me/s/durov). Пусто — совпадает с запрошенным.
	URL      string
	Title    string
	Articles []Article
}

// Delivery — новость, которую нужно доставить подписчику.
type Delivery struct {
	ChatID  int64
	Article Article
}

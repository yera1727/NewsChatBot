package rss

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"PetProjectClaude/internal/adapter/safehttp"
)

// Клиент RSS должен использовать защищённый HTTP-клиент.
func TestFetch_LocalServerRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`<rss><channel><title>Секрет</title></channel></rss>`))
	}))
	defer srv.Close()

	_, err := NewClient(5*time.Second).Fetch(context.Background(), srv.URL)

	if !errors.Is(err, safehttp.ErrForbiddenAddress) {
		t.Errorf("ожидали отказ из-за локального адреса, получили %v", err)
	}
}

func TestFetch_ParsesFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0"><channel>
  <title> Тестовая лента </title>
  <item>
    <title>Первая</title>
    <link>https://example.com/1</link>
    <pubDate>Tue, 10 Nov 2026 18:24:41 +0000</pubDate>
  </item>
  <item><title>Вторая</title><link>https://example.com/2</link></item>
</channel></rss>`))
	}))
	defer srv.Close()

	c := NewClient(5 * time.Second)
	safehttp.AllowLocalForTests(c.http) // тестовый сервер на 127.0.0.1

	feed, err := c.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if feed.Title != "Тестовая лента" || len(feed.Articles) != 2 {
		t.Fatalf("ожидали «Тестовая лента» с 2 статьями, получили %q и %d", feed.Title, len(feed.Articles))
	}
	if feed.Articles[0].PublishedAt.IsZero() || !feed.Articles[1].PublishedAt.IsZero() {
		t.Errorf("дата должна разобраться у первой статьи и отсутствовать у второй")
	}
}

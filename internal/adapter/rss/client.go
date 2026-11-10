// Package rss — адаптер: получает новости из RSS-лент.
// Реализует интерфейс usecase.FeedSource.
package rss

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"time"

	"PetProjectClaude/internal/domain"
)

// rssItem и rssFeed повторяют структуру XML. Это детали формата RSS,
// поэтому они не экспортируются и не попадают в domain.
type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

type rssFeed struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

// Client скачивает RSS-ленты.
type Client struct {
	http *http.Client
}

func NewClient(timeout time.Duration) *Client {
	return &Client{http: &http.Client{Timeout: timeout}}
}

// Fetch скачивает ленту и переводит её в доменные Article.
func (c *Client) Fetch(ctx context.Context, feedURL string) ([]domain.Article, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("создание запроса: %w", err)
	}
	req.Header.Set("User-Agent", "newsbot/0.1") // некоторые сайты без него отказывают

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("запрос %s: %w", feedURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("запрос %s: статус %s", feedURL, resp.Status)
	}

	var feed rssFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("разбор XML %s: %w", feedURL, err)
	}

	// Преобразуем формат RSS → доменную модель.
	articles := make([]domain.Article, 0, len(feed.Channel.Items))
	for _, it := range feed.Channel.Items {
		articles = append(articles, domain.Article{
			Source:      feedURL,
			Title:       it.Title,
			Link:        it.Link,
			Description: it.Description,
			PublishedAt: parseDate(it.PubDate),
		})
	}
	return articles, nil
}

// parseDate пробует форматы дат, которые встречаются в RSS.
// Если ни один не подошёл, возвращает нулевое время.
func parseDate(s string) time.Time {
	for _, layout := range []string{time.RFC1123Z, time.RFC1123} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

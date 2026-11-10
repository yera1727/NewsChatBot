// Package rss — адаптер: получает новости из RSS-лент.
// Реализует интерфейс usecase.FeedFetcher.
package rss

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
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
		Title string    `xml:"title"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

// maxFeedSize — сколько байт максимум читаем из ответа. Защита от огромных
// (или бесконечных) ответов, которые съели бы всю память.
const maxFeedSize = 10 << 20 // 10 МБ

var errForbiddenAddress = errors.New("адреса внутренней сети запрещены")

// Client скачивает RSS-ленты.
type Client struct {
	http *http.Client
}

func NewClient(timeout time.Duration) *Client {
	// Ленты теперь добавляют пользователи, поэтому запрещаем ходить во внутреннюю сеть
	// (localhost, 10.x, 192.168.x, 169.254.x и т.п.). Иначе через бота можно было бы
	// обращаться к сервисам, закрытым от интернета, — это атака SSRF.
	//
	// Проверка стоит в Control: он вызывается для КАЖДОГО соединения уже после
	// DNS-резолва, поэтому её не обойти ни редиректом, ни DNS-записью на 127.0.0.1.
	dialer := &net.Dialer{Timeout: 5 * time.Second, Control: blockInternalAddresses}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialer.DialContext
	transport.Proxy = nil // через прокси проверялся бы адрес прокси, а не сайта

	return &Client{http: &http.Client{Timeout: timeout, Transport: transport}}
}

// blockInternalAddresses запрещает соединения с локальными и внутренними IP.
func blockInternalAddresses(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return fmt.Errorf("%w: %s", errForbiddenAddress, host)
	}
	return nil
}

// Fetch скачивает ленту и переводит её в доменную модель.
// Реализует usecase.FeedFetcher. Про источники в нашей БД адаптер ничего не знает —
// привязку статей к источнику делает use case.
func (c *Client) Fetch(ctx context.Context, feedURL string) (domain.Feed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return domain.Feed{}, fmt.Errorf("создание запроса: %w", err)
	}
	req.Header.Set("User-Agent", "newsbot/0.1") // некоторые сайты без него отказывают

	resp, err := c.http.Do(req)
	if err != nil {
		return domain.Feed{}, fmt.Errorf("запрос %s: %w", feedURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return domain.Feed{}, fmt.Errorf("запрос %s: статус %s", feedURL, resp.Status)
	}

	var feed rssFeed
	if err := xml.NewDecoder(io.LimitReader(resp.Body, maxFeedSize)).Decode(&feed); err != nil {
		return domain.Feed{}, fmt.Errorf("разбор XML %s: %w", feedURL, err)
	}

	// Преобразуем формат RSS → доменную модель.
	articles := make([]domain.Article, 0, len(feed.Channel.Items))
	for _, it := range feed.Channel.Items {
		articles = append(articles, domain.Article{
			Title:       strings.TrimSpace(it.Title),
			Link:        strings.TrimSpace(it.Link),
			Description: it.Description,
			PublishedAt: parseDate(it.PubDate),
		})
	}
	return domain.Feed{Title: strings.TrimSpace(feed.Channel.Title), Articles: articles}, nil
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

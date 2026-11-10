// Package tgchannel — адаптер: читает посты публичных Telegram-каналов.
// Реализует интерфейс usecase.FeedFetcher.
//
// Telegram-бот не может читать чужие каналы через Bot API, поэтому используем
// веб-превью, которое есть у каждого публичного канала: https://t.me/s/<имя>.
// Это HTML-страница с последними ~20 постами; мы разбираем её и превращаем
// посты в доменные Article — дальше они живут так же, как статьи из RSS.
//
// Важно: это разбор вёрстки, а не официальный API. Если Telegram поменяет
// HTML-классы, нужно будет поправить селекторы в parsePage.
package tgchannel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"

	"PetProjectClaude/internal/adapter/safehttp"
	"PetProjectClaude/internal/domain"
)

// ErrNotPublicChannel — ссылка на t.me, но не на публичный канал
// (приглашение t.me/+..., ссылка на бота, некорректное имя).
var ErrNotPublicChannel = errors.New("поддерживаются только публичные каналы вида t.me/имя")

// Имя канала в Telegram: 5–32 символа, латиница, цифры и _, начинается с буквы.
// Берём от 4 символов с запасом: у старых каналов бывают короткие имена.
var channelName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{3,31}$`)

const maxTitleLen = 100 // столько символов поста показываем как заголовок

// IsTelegramURL — ведёт ли ссылка на Telegram (t.me или telegram.me).
func IsTelegramURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	return host == "t.me" || host == "telegram.me"
}

// ChannelFromURL достаёт имя канала из ссылки. Понимает все популярные формы:
//
//	https://t.me/durov, https://t.me/s/durov, https://t.me/durov/554 (ссылка на пост)
func ChannelFromURL(raw string) (string, bool) {
	if !IsTelegramURL(raw) {
		return "", false
	}
	u, _ := url.Parse(raw)
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 && parts[0] == "s" {
		parts = parts[1:]
	}
	if len(parts) == 0 || !channelName.MatchString(parts[0]) {
		return "", false
	}
	// Имена каналов не зависят от регистра: t.me/Durov и t.me/durov — один канал.
	return strings.ToLower(parts[0]), true
}

// ChannelURL — канонический адрес канала, под которым он хранится в БД.
func ChannelURL(name string) string {
	return "https://t.me/s/" + name
}

type Client struct {
	http *http.Client
}

func NewClient(timeout time.Duration) *Client {
	return &Client{http: safehttp.NewClient(timeout)}
}

// Fetch скачивает веб-превью канала и возвращает его посты.
// В Feed.URL кладёт канонический адрес канала, чтобы разные формы одной
// ссылки (t.me/durov, t.me/s/durov, @durov) не стали разными источниками.
func (c *Client) Fetch(ctx context.Context, rawURL string) (domain.Feed, error) {
	name, ok := ChannelFromURL(rawURL)
	if !ok {
		return domain.Feed{}, ErrNotPublicChannel
	}
	pageURL := ChannelURL(name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return domain.Feed{}, fmt.Errorf("создание запроса: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; newsbot/0.1)")

	resp, err := c.http.Do(req)
	if err != nil {
		return domain.Feed{}, fmt.Errorf("запрос %s: %w", pageURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return domain.Feed{}, fmt.Errorf("запрос %s: статус %s", pageURL, resp.Status)
	}

	feed, err := parsePage(io.LimitReader(resp.Body, safehttp.MaxBodySize))
	if err != nil {
		return domain.Feed{}, fmt.Errorf("разбор %s: %w", pageURL, err)
	}
	// Несуществующий или закрытый канал Telegram отдаёт страницей без постов —
	// use case увидит пустую ленту и вернёт пользователю понятную ошибку.
	feed.URL = pageURL
	return feed, nil
}

// parsePage разбирает HTML веб-превью канала. Структура одного поста (упрощённо):
//
//	<div class="tgme_widget_message" data-post="durov/554">
//	  <div class="tgme_widget_message_bubble">
//	    <a class="tgme_widget_message_reply">...цитата...</a>    ← пропускаем
//	    <div class="media_supported_cont">                        ← у постов с фото/видео
//	      <a class="tgme_widget_message_photo_wrap">
//	      <div class="tgme_widget_message_text">Подпись</div>
//	    </div>
//	    <div class="tgme_widget_message_text">Текст<br/>поста</div> ← у текстовых постов
//	    <a class="tgme_widget_message_date" href="https://t.me/durov/554">
//	      <time datetime="2026-11-10T12:59:06+00:00">
func parsePage(r io.Reader) (domain.Feed, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return domain.Feed{}, err
	}

	feed := domain.Feed{
		Title: strings.TrimSpace(doc.Find(`meta[property="og:title"]`).AttrOr("content", "")),
	}

	doc.Find(".tgme_widget_message[data-post]").Each(func(_ int, msg *goquery.Selection) {
		date := msg.Find("a.tgme_widget_message_date")
		link := date.AttrOr("href", "")
		if link == "" {
			return // без ссылки пост не сохранить: по ней отсекаются дубли
		}

		// Текст поста может лежать на разной глубине (у фото и видео — внутри
		// media_supported_cont), поэтому ищем везде, кроме цитаты (reply) — там
		// такой же класс, но это текст чужого сообщения.
		textBlock := msg.Find(".tgme_widget_message_text").FilterFunction(func(_ int, s *goquery.Selection) bool {
			return s.Closest(".tgme_widget_message_reply").Length() == 0
		}).First()
		text := messageText(textBlock)

		title := firstLine(text, maxTitleLen)
		if title == "" {
			title = mediaTitle(msg)
		}

		published, _ := time.Parse(time.RFC3339, date.Find("time").AttrOr("datetime", ""))

		feed.Articles = append(feed.Articles, domain.Article{
			Title:       title,
			Link:        link,
			Description: text,
			PublishedAt: published, // если дата не разобралась — нулевое время, в БД будет NULL
		})
	})

	return feed, nil
}

// messageText достаёт текст поста, сохраняя переносы строк (<br> → \n).
func messageText(sel *goquery.Selection) string {
	sel.Find("br").ReplaceWithHtml("\n")
	return strings.TrimSpace(sel.Text())
}

// firstLine — первая непустая строка текста, обрезанная до max символов.
func firstLine(text string, max int) string {
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if utf8.RuneCountInString(line) > max {
			// Режем по символам (рунам), а не по байтам — иначе разрежем букву пополам.
			line = string([]rune(line)[:max]) + "…"
		}
		return line
	}
	return ""
}

// mediaTitle — заголовок для поста без текста.
func mediaTitle(msg *goquery.Selection) string {
	switch {
	case msg.Find(".tgme_widget_message_photo_wrap").Length() > 0:
		return "📷 Фото"
	case msg.Find(".tgme_widget_message_video_player, .tgme_widget_message_roundvideo_player").Length() > 0:
		return "🎬 Видео"
	default:
		return "Пост без текста"
	}
}

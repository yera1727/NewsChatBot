package feedrouter

import (
	"context"
	"testing"

	"PetProjectClaude/internal/domain"
)

// named возвращает ленту со своим именем — по нему видно, какой адаптер вызван.
type named string

func (n named) Fetch(context.Context, string) (domain.Feed, error) {
	return domain.Feed{Title: string(n)}, nil
}

func TestRouter(t *testing.T) {
	r := New(named("rss"), named("telegram"))

	for url, want := range map[string]string{
		"https://t.me/durov":              "telegram",
		"https://telegram.me/s/durov":     "telegram",
		"https://example.com/rss":         "rss",
		"https://news.example.org/feed":   "rss",
		"https://example.com/t.me/durov/": "rss", // t.me в пути, а не в домене
	} {
		feed, _ := r.Fetch(context.Background(), url)
		if feed.Title != want {
			t.Errorf("%s: ожидали адаптер %s, вызван %s", url, want, feed.Title)
		}
	}
}

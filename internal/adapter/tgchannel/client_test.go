package tgchannel

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestChannelFromURL(t *testing.T) {
	tests := []struct {
		url    string
		want   string
		wantOK bool
	}{
		{"https://t.me/durov", "durov", true},
		{"https://t.me/s/durov", "durov", true},
		{"https://t.me/durov/554", "durov", true}, // ссылка на пост
		{"https://t.me/Durov", "durov", true},     // регистр не важен
		{"https://telegram.me/durov", "durov", true},
		{"https://www.t.me/durov/", "durov", true},
		{"https://t.me/+AbCdEf123", "", false},  // приглашение в закрытый канал
		{"https://t.me/joinchat/xyz", "", true}, // похоже на имя — Telegram сам вернёт пустую страницу
		{"https://t.me/", "", false},
		{"https://t.me/1abc", "", false}, // имя не может начинаться с цифры
		{"https://example.com/durov", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got, ok := ChannelFromURL(tt.url)
			if ok != tt.wantOK || (tt.wantOK && tt.want != "" && got != tt.want) {
				t.Errorf("получили (%q, %v), ожидали (%q, %v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestFetch_NotPublicChannel(t *testing.T) {
	_, err := NewClient(time.Second).Fetch(t.Context(), "https://t.me/+AbCdEf123")
	if !errors.Is(err, ErrNotPublicChannel) {
		t.Errorf("ожидали ErrNotPublicChannel, получили %v", err)
	}
}

// Упрощённая копия реальной вёрстки t.me/s/<канал>.
const samplePage = `<!DOCTYPE html><html><head>
<meta property="og:title" content="Тестовый канал">
</head><body>

<div class="tgme_widget_message_wrap"><div class="tgme_widget_message" data-post="test/1">
  <div class="tgme_widget_message_bubble">
    <div class="tgme_widget_message_text js-message_text">Первая строка — заголовок<br/><br/>Вторая строка <i class="emoji"><b>⚡️</b></i></div>
    <div class="tgme_widget_message_footer">
      <a class="tgme_widget_message_date" href="https://t.me/test/1"><time datetime="2026-11-10T12:59:06+00:00" class="time">12:59</time></a>
    </div>
  </div>
</div></div>

<div class="tgme_widget_message_wrap"><div class="tgme_widget_message" data-post="test/2">
  <div class="tgme_widget_message_bubble">
    <a class="tgme_widget_message_reply" href="https://t.me/test/1">
      <div class="tgme_widget_message_text js-message_reply_text">Текст цитаты — его брать нельзя</div>
    </a>
    <a class="tgme_widget_message_photo_wrap" href="https://t.me/test/2"></a>
    <div class="tgme_widget_message_footer">
      <a class="tgme_widget_message_date" href="https://t.me/test/2"><time datetime="2026-11-10T13:00:00+00:00">13:00</time></a>
    </div>
  </div>
</div></div>

<div class="tgme_widget_message_wrap"><div class="tgme_widget_message" data-post="test/3">
  <div class="tgme_widget_message_bubble">
    <div class="media_supported_cont">
      <a class="tgme_widget_message_photo_wrap" href="https://t.me/test/3"></a>
      <div class="tgme_widget_message_text js-message_text"><b>700K+</b> кошельков за 48 часов</div>
    </div>
    <div class="tgme_widget_message_footer">
      <a class="tgme_widget_message_date" href="https://t.me/test/3"><time datetime="2026-11-10T14:00:00+00:00">14:00</time></a>
    </div>
  </div>
</div></div>

</body></html>`

func TestParsePage(t *testing.T) {
	feed, err := parsePage(strings.NewReader(samplePage))
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	if feed.Title != "Тестовый канал" {
		t.Errorf("название канала: %q", feed.Title)
	}
	if len(feed.Articles) != 3 {
		t.Fatalf("ожидали 3 поста, получили %d", len(feed.Articles))
	}

	first := feed.Articles[0]
	if first.Title != "Первая строка — заголовок" {
		t.Errorf("заголовок — первая строка текста, получили %q", first.Title)
	}
	if first.Link != "https://t.me/test/1" {
		t.Errorf("ссылка на пост: %q", first.Link)
	}
	if !strings.Contains(first.Description, "\n") || !strings.Contains(first.Description, "⚡️") {
		t.Errorf("в тексте должны сохраниться переносы строк и эмодзи: %q", first.Description)
	}
	if want := time.Date(2026, 11, 10, 12, 59, 6, 0, time.UTC); !first.PublishedAt.Equal(want) {
		t.Errorf("дата: %v", first.PublishedAt)
	}

	second := feed.Articles[1]
	if second.Title != "📷 Фото" {
		t.Errorf("пост только с фото должен называться «📷 Фото», получили %q", second.Title)
	}
	if strings.Contains(second.Description, "цитаты") {
		t.Errorf("текст цитаты попал в пост: %q", second.Description)
	}

	// Подпись к фото лежит внутри media_supported_cont — её тоже нужно найти.
	if third := feed.Articles[2]; third.Title != "700K+ кошельков за 48 часов" {
		t.Errorf("заголовком поста с фото должна быть подпись, получили %q", third.Title)
	}
}

func TestFirstLine_TruncatesByRunes(t *testing.T) {
	long := strings.Repeat("я", 150) // кириллица: 2 байта на букву
	got := firstLine("\n\n"+long, 100)
	if got != strings.Repeat("я", 100)+"…" {
		t.Errorf("ожидали 100 букв и многоточие, получили %d символов", len([]rune(got)))
	}
}

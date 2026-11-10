// Package telegram — адаптер: Telegram-бот.
//
// Он работает в обе стороны:
//   - входящие команды пользователя → вызов use case'ов → ответ;
//   - реализует usecase.Notifier, чтобы use case рассылки мог отправлять сообщения.
//
// Вся «телеграмная» специфика (HTML-разметка, команды, коды ошибок API) живёт здесь,
// use case'ы о ней не знают.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"PetProjectClaude/internal/domain"
	"PetProjectClaude/internal/usecase"
)

const latestLimit = 10

const helpText = `Привет! Я собираю новости из RSS-лент.

/latest — последние новости
/sources — список источников
/subscribe N — подписаться на источник номер N
/unsubscribe N — отписаться
/my — мои подписки
/add ссылка — добавить RSS-ленту или Telegram-канал (@имя)`

// UseCases — сценарии, которые вызывает бот. Собраны в структуру, чтобы
// конструктор не разрастался с каждой новой командой.
type UseCases struct {
	Latest        *usecase.LatestNews
	Subscriptions *usecase.Subscriptions
	AddSource     *usecase.AddSource
}

type Bot struct {
	api    *bot.Bot
	token  string
	uc     UseCases
	logger *slog.Logger
}

// New создаёт бота. Библиотека сразу вызывает метод getMe,
// так что неверный токен обнаружится здесь, при старте.
func New(token string, uc UseCases, logger *slog.Logger) (*Bot, error) {
	b := &Bot{token: token, uc: uc, logger: logger}

	api, err := bot.New(token, bot.WithDefaultHandler(b.handle))
	if err != nil {
		return nil, fmt.Errorf("создание бота: %w", b.cleanErr(err))
	}
	b.api = api
	return b, nil
}

// Start получает сообщения от Telegram (long polling) и блокируется до отмены ctx.
func (b *Bot) Start(ctx context.Context) {
	b.api.Start(ctx)
}

// Notify отправляет новость в чат. Реализует usecase.Notifier.
func (b *Bot) Notify(ctx context.Context, chatID int64, a domain.Article) error {
	text := fmt.Sprintf("<b>%s</b>\n\n<a href=\"%s\">Читать на %s</a>",
		html.EscapeString(a.Title), html.EscapeString(a.Link), sourceName(a.Source))

	err := b.send(ctx, chatID, text, true)
	if errors.Is(err, bot.ErrorForbidden) {
		// 403 от Telegram = пользователь заблокировал бота.
		// Переводим ошибку библиотеки в ошибку бизнес-логики.
		return fmt.Errorf("%w: %w", usecase.ErrChatUnavailable, err)
	}
	return err
}

// handle вызывается библиотекой на каждое входящее обновление.
func (b *Bot) handle(ctx context.Context, _ *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.Text == "" {
		return // нас интересуют только текстовые сообщения
	}
	chatID := update.Message.Chat.ID
	cmd, arg := parseCommand(update.Message.Text)

	var (
		reply string
		err   error
	)
	switch cmd {
	case "/start", "/help":
		reply = helpText
	case "/latest":
		reply, err = b.latest(ctx)
	case "/sources":
		reply, err = b.sources(ctx)
	case "/subscribe":
		reply, err = b.subscribe(ctx, chatID, arg)
	case "/unsubscribe":
		reply, err = b.unsubscribe(ctx, chatID, arg)
	case "/my":
		reply, err = b.my(ctx, chatID)
	case "/add":
		reply, err = b.add(ctx, chatID, arg)
	default:
		reply = "Не знаю такой команды. Список команд: /help"
	}

	if err != nil {
		b.logger.Error("обработка команды", "cmd", cmd, "chat", chatID, "err", err)
		reply = "Что-то пошло не так, попробуйте позже."
	}
	if err := b.send(ctx, chatID, reply, false); err != nil {
		b.logger.Error("отправка ответа", "chat", chatID, "err", err)
	}
}

func (b *Bot) latest(ctx context.Context) (string, error) {
	articles, err := b.uc.Latest.Run(ctx, latestLimit)
	if err != nil {
		return "", err
	}
	if len(articles) == 0 {
		return "Новостей пока нет — fetcher ещё ничего не собрал.", nil
	}

	var sb strings.Builder
	sb.WriteString("<b>Последние новости</b>\n\n")
	for i, a := range articles {
		fmt.Fprintf(&sb, "%d. <a href=\"%s\">%s</a> · %s\n",
			i+1, html.EscapeString(a.Link), html.EscapeString(a.Title), sourceName(a.Source))
	}
	return sb.String(), nil
}

func (b *Bot) sources(ctx context.Context) (string, error) {
	sources, err := b.uc.Subscriptions.Sources(ctx)
	if err != nil {
		return "", err
	}
	if len(sources) == 0 {
		return "Источников пока нет.", nil
	}

	var sb strings.Builder
	sb.WriteString("<b>Источники</b>\n\n")
	// Номер — это ID источника в БД: он не меняется, когда добавляются новые источники.
	for _, s := range sources {
		fmt.Fprintf(&sb, "%d. %s\n", s.ID, sourceName(s))
	}
	fmt.Fprintf(&sb, "\nПодписаться: /subscribe %d", sources[0].ID)
	return sb.String(), nil
}

func (b *Bot) subscribe(ctx context.Context, chatID int64, arg string) (string, error) {
	n, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return "Укажите номер источника из /sources, например: /subscribe 1", nil
	}
	source, err := b.uc.Subscriptions.Subscribe(ctx, chatID, n)
	if errors.Is(err, usecase.ErrUnknownSource) {
		return fmt.Sprintf("Нет источника с номером %d. Список: /sources", n), nil
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("✅ Вы подписаны на %s. Новые статьи будут приходить сюда.", sourceName(source)), nil
}

func (b *Bot) unsubscribe(ctx context.Context, chatID int64, arg string) (string, error) {
	n, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return "Укажите номер источника из /my, например: /unsubscribe 1", nil
	}
	source, err := b.uc.Subscriptions.Unsubscribe(ctx, chatID, n)
	if errors.Is(err, usecase.ErrUnknownSource) {
		return fmt.Sprintf("Нет источника с номером %d. Список: /sources", n), nil
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Вы отписались от %s.", sourceName(source)), nil
}

func (b *Bot) my(ctx context.Context, chatID int64) (string, error) {
	sources, err := b.uc.Subscriptions.List(ctx, chatID)
	if err != nil {
		return "", err
	}
	if len(sources) == 0 {
		return "Вы пока ни на что не подписаны. Список источников: /sources", nil
	}

	var sb strings.Builder
	sb.WriteString("<b>Ваши подписки</b>\n\n")
	for _, s := range sources {
		fmt.Fprintf(&sb, "%d. %s\n", s.ID, sourceName(s))
	}
	fmt.Fprintf(&sb, "\nОтписаться: /unsubscribe %d", sources[0].ID)
	return sb.String(), nil
}

func (b *Bot) add(ctx context.Context, chatID int64, arg string) (string, error) {
	if arg == "" {
		return "Пришлите ссылку на RSS-ленту или Telegram-канал, например:\n" +
			"/add https://habr.com/ru/rss/news/\n/add @durov", nil
	}

	// «@канал» — привычная в Telegram запись, превращаем её в ссылку.
	if name, ok := strings.CutPrefix(arg, "@"); ok {
		arg = "https://t.me/" + name
	}

	src, created, err := b.uc.AddSource.Run(ctx, chatID, arg)
	switch {
	case errors.Is(err, usecase.ErrInvalidURL):
		return "Это не похоже на ссылку. Нужен адрес вида https://example.com/rss", nil
	case errors.Is(err, usecase.ErrNotAFeed):
		// Причину пишем в лог, а пользователю — понятное объяснение.
		b.logger.Info("не удалось добавить ленту", "chat", chatID, "url", arg, "err", err)
		return "Не получилось прочитать ленту по этой ссылке.\n\n" +
			"• Для сайта нужна именно RSS-лента, а не обычная страница (часто она лежит по адресу вида /rss или /feed).\n" +
			"• Для Telegram подходят только публичные каналы с постами: @имя или t.me/имя.", nil
	case errors.Is(err, usecase.ErrSourceDisabled):
		return "Этот источник отключён и недоступен для подписки.", nil
	case err != nil:
		return "", err
	}

	if created {
		return fmt.Sprintf("✅ Добавлен новый источник: <b>%s</b> (номер %d).\nВы на него подписаны — новые статьи будут приходить сюда.",
			sourceName(src), src.ID), nil
	}
	return fmt.Sprintf("Источник <b>%s</b> уже есть (номер %d). Вы на него подписаны.", sourceName(src), src.ID), nil
}

// send отправляет HTML-сообщение. preview — показывать ли превью ссылки.
func (b *Bot) send(ctx context.Context, chatID int64, text string, preview bool) error {
	params := &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	}
	if !preview {
		params.LinkPreviewOptions = &models.LinkPreviewOptions{IsDisabled: bot.True()}
	}
	_, err := b.api.SendMessage(ctx, params)
	return b.cleanErr(err)
}

// parseCommand разбирает "/subscribe@MyBot 2" → ("/subscribe", "2").
// Суффикс @ИмяБота Telegram добавляет к командам в групповых чатах.
func parseCommand(text string) (cmd, arg string) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", ""
	}
	cmd, _, _ = strings.Cut(fields[0], "@")
	if len(fields) > 1 {
		arg = fields[1]
	}
	return strings.ToLower(cmd), arg
}

// sourceName — имя источника, безопасное для вставки в HTML-сообщение.
func sourceName(s domain.Source) string {
	return html.EscapeString(s.Name())
}

// cleanErr убирает токен из текста ошибки. Сетевые ошибки net/http содержат
// полный URL запроса (https://api.telegram.org/bot<ТОКЕН>/...) — и токен утёк бы в логи.
func (b *Bot) cleanErr(err error) error {
	if err == nil || !strings.Contains(err.Error(), b.token) {
		return err
	}
	return &redactedError{msg: strings.ReplaceAll(err.Error(), b.token, "<TOKEN>"), err: err}
}

// redactedError подменяет текст ошибки, но сохраняет цепочку для errors.Is.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

package usecase

import (
	"context"
	"errors"
	"testing"

	"PetProjectClaude/internal/domain"
)

// --- фейки ---

// fakeSourceReader хранит источники по ID, как таблица sources.
type fakeSourceReader struct {
	byID map[int64]domain.Source
}

func (f *fakeSourceReader) Sources(context.Context) ([]domain.Source, error) { return nil, nil }
func (f *fakeSourceReader) SourceByID(_ context.Context, id int64) (domain.Source, error) {
	src, ok := f.byID[id]
	if !ok {
		return domain.Source{}, ErrUnknownSource
	}
	return src, nil
}

type fakeSubRepo struct {
	subscribed     map[int64][]int64 // chatID → id источников
	pending        []domain.Delivery
	delivered      []int64 // id доставленных статей
	unsubscribeAll []int64 // чаты, отписанные целиком
}

func (f *fakeSubRepo) Subscribe(_ context.Context, chatID, sourceID int64) error {
	if f.subscribed == nil {
		f.subscribed = make(map[int64][]int64)
	}
	f.subscribed[chatID] = append(f.subscribed[chatID], sourceID)
	return nil
}
func (f *fakeSubRepo) Unsubscribe(context.Context, int64, int64) error { return nil }
func (f *fakeSubRepo) UnsubscribeAll(_ context.Context, chatID int64) error {
	f.unsubscribeAll = append(f.unsubscribeAll, chatID)
	return nil
}
func (f *fakeSubRepo) ListByChat(_ context.Context, chatID int64) ([]domain.Source, error) {
	var list []domain.Source
	for _, id := range f.subscribed[chatID] {
		list = append(list, domain.Source{ID: id})
	}
	return list, nil
}
func (f *fakeSubRepo) Pending(context.Context, int) ([]domain.Delivery, error) {
	return f.pending, nil
}
func (f *fakeSubRepo) MarkDelivered(_ context.Context, _, _, articleID int64) error {
	f.delivered = append(f.delivered, articleID)
	return nil
}

// fakeNotifier возвращает заданную ошибку для конкретных чатов.
type fakeNotifier struct {
	errFor map[int64]error
	sent   []int64 // id отправленных статей
}

func (f *fakeNotifier) Notify(_ context.Context, chatID int64, a domain.Article) error {
	if err := f.errFor[chatID]; err != nil {
		return err
	}
	f.sent = append(f.sent, a.ID)
	return nil
}

// fakeArticleReader запоминает, для какого чата запросили новости.
type fakeArticleReader struct {
	articles  []domain.Article
	askedChat int64
}

func (f *fakeArticleReader) LatestForChat(_ context.Context, chatID int64, _ int) ([]domain.Article, error) {
	f.askedChat = chatID
	return f.articles, nil
}

// --- тесты последних новостей ---

func TestLatestNews_NoSubscriptions(t *testing.T) {
	reader := &fakeArticleReader{}
	_, err := NewLatestNews(reader, &fakeSubRepo{}).Run(context.Background(), 42, 10)

	if !errors.Is(err, ErrNoSubscriptions) {
		t.Errorf("без подписок ожидали ErrNoSubscriptions, получили %v", err)
	}
	if reader.askedChat != 0 {
		t.Error("без подписок статьи запрашивать незачем")
	}
}

func TestLatestNews_ForSubscribedChat(t *testing.T) {
	reader := &fakeArticleReader{articles: []domain.Article{{ID: 1}, {ID: 2}}}
	subs := &fakeSubRepo{}
	_ = subs.Subscribe(context.Background(), 42, 7)

	got, err := NewLatestNews(reader, subs).Run(context.Background(), 42, 10)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if reader.askedChat != 42 || len(got) != 2 {
		t.Errorf("ожидали 2 статьи для чата 42, получили %d для чата %d", len(got), reader.askedChat)
	}
}

// --- тесты подписок ---

func testSources() *fakeSourceReader {
	return &fakeSourceReader{byID: map[int64]domain.Source{
		1: {ID: 1, Title: "Хабр", Active: true},
		2: {ID: 2, Title: "Lenta.ru", Active: true},
		3: {ID: 3, Title: "Закрытая лента", Active: false},
	}}
}

func TestSubscriptions_SubscribeByID(t *testing.T) {
	repo := &fakeSubRepo{}
	uc := NewSubscriptions(testSources(), repo)

	source, err := uc.Subscribe(context.Background(), 42, 2)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if source.Title != "Lenta.ru" {
		t.Errorf("источник 2 должен быть Lenta.ru, получили %q", source.Title)
	}
	if got := repo.subscribed[42]; len(got) != 1 || got[0] != 2 {
		t.Errorf("в хранилище должна быть подписка на источник 2, получили %v", got)
	}
}

func TestSubscriptions_UnknownOrInactiveSource(t *testing.T) {
	uc := NewSubscriptions(testSources(), &fakeSubRepo{})

	// Табличный тест — идиома Go: один код проверки, много входных данных.
	// 3 — существует, но неактивен; на него тоже нельзя подписаться.
	for _, id := range []int64{0, -1, 3, 100} {
		if _, err := uc.Subscribe(context.Background(), 42, id); !errors.Is(err, ErrUnknownSource) {
			t.Errorf("источник %d: ожидали ErrUnknownSource, получили %v", id, err)
		}
	}
}

func TestSubscriptions_CanUnsubscribeFromInactive(t *testing.T) {
	uc := NewSubscriptions(testSources(), &fakeSubRepo{})

	if _, err := uc.Unsubscribe(context.Background(), 42, 3); err != nil {
		t.Errorf("от неактивного источника отписаться можно, получили ошибку %v", err)
	}
}

// --- тесты рассылки ---

func TestNotify_BlockedChatIsUnsubscribed_OthersStillReceive(t *testing.T) {
	const blocked, normal = 1, 2
	repo := &fakeSubRepo{pending: []domain.Delivery{
		{ChatID: blocked, Article: domain.Article{ID: 10}},
		{ChatID: normal, Article: domain.Article{ID: 11}},
		{ChatID: blocked, Article: domain.Article{ID: 12}},
	}}
	notifier := &fakeNotifier{errFor: map[int64]error{blocked: ErrChatUnavailable}}

	sent, err := NewNotifySubscribers(repo, notifier, 50).Run(context.Background())

	if err != nil {
		t.Errorf("блокировка бота — не ошибка рассылки, получили %v", err)
	}
	if sent != 1 {
		t.Errorf("ожидали 1 доставленное сообщение, получили %d", sent)
	}
	if len(repo.unsubscribeAll) != 1 || repo.unsubscribeAll[0] != blocked {
		t.Errorf("заблокировавший чат должен быть отписан один раз, получили %v", repo.unsubscribeAll)
	}
	if len(repo.delivered) != 1 || repo.delivered[0] != 11 {
		t.Errorf("доставленной должна быть отмечена только статья 11, получили %v", repo.delivered)
	}
}

func TestNotify_FailedSendIsNotMarkedDelivered(t *testing.T) {
	repo := &fakeSubRepo{pending: []domain.Delivery{
		{ChatID: 1, Article: domain.Article{ID: 5}},
		{ChatID: 1, Article: domain.Article{ID: 6}},
	}}
	notifier := &fakeNotifier{errFor: map[int64]error{1: errors.New("сеть недоступна")}}

	sent, err := NewNotifySubscribers(repo, notifier, 50).Run(context.Background())

	if err == nil {
		t.Error("ожидали ошибку отправки")
	}
	if sent != 0 || len(repo.delivered) != 0 {
		t.Errorf("ничего не должно быть отмечено доставленным: sent=%d delivered=%v", sent, repo.delivered)
	}
}

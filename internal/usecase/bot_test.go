package usecase

import (
	"context"
	"errors"
	"testing"

	"PetProjectClaude/internal/domain"
)

// --- фейки ---

type fakeReader struct {
	sources []string
}

func (f *fakeReader) Latest(context.Context, int) ([]domain.Article, error) { return nil, nil }
func (f *fakeReader) Sources(context.Context) ([]string, error)             { return f.sources, nil }

type fakeSubRepo struct {
	subscribed     map[int64][]string
	pending        []domain.Delivery
	delivered      []int64 // id доставленных статей
	unsubscribeAll []int64 // чаты, отписанные целиком
}

func (f *fakeSubRepo) Subscribe(_ context.Context, chatID int64, source string) error {
	if f.subscribed == nil {
		f.subscribed = make(map[int64][]string)
	}
	f.subscribed[chatID] = append(f.subscribed[chatID], source)
	return nil
}
func (f *fakeSubRepo) Unsubscribe(context.Context, int64, string) error { return nil }
func (f *fakeSubRepo) UnsubscribeAll(_ context.Context, chatID int64) error {
	f.unsubscribeAll = append(f.unsubscribeAll, chatID)
	return nil
}
func (f *fakeSubRepo) ListByChat(_ context.Context, chatID int64) ([]string, error) {
	return f.subscribed[chatID], nil
}
func (f *fakeSubRepo) Pending(context.Context, int) ([]domain.Delivery, error) {
	return f.pending, nil
}
func (f *fakeSubRepo) MarkDelivered(_ context.Context, _ int64, _ string, articleID int64) error {
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

// --- тесты подписок ---

func TestSubscriptions_SubscribeByNumber(t *testing.T) {
	repo := &fakeSubRepo{}
	uc := NewSubscriptions(&fakeReader{sources: []string{"habr", "lenta"}}, repo)

	source, err := uc.Subscribe(context.Background(), 42, 2)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if source != "lenta" {
		t.Errorf("номер 2 должен быть lenta, получили %q", source)
	}
	if got := repo.subscribed[42]; len(got) != 1 || got[0] != "lenta" {
		t.Errorf("в хранилище должна быть подписка на lenta, получили %v", got)
	}
}

func TestSubscriptions_UnknownNumber(t *testing.T) {
	uc := NewSubscriptions(&fakeReader{sources: []string{"habr"}}, &fakeSubRepo{})

	// Табличный тест — идиома Go: один код проверки, много входных данных.
	for _, n := range []int{0, -1, 2, 100} {
		if _, err := uc.Subscribe(context.Background(), 42, n); !errors.Is(err, ErrUnknownSource) {
			t.Errorf("номер %d: ожидали ErrUnknownSource, получили %v", n, err)
		}
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

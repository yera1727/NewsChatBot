package safehttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBlockInternalAddresses(t *testing.T) {
	tests := []struct {
		addr    string
		blocked bool
	}{
		{"127.0.0.1:5433", true},       // localhost — наша БД
		{"[::1]:80", true},             // localhost IPv6
		{"10.0.0.5:80", true},          // частная сеть
		{"192.168.1.1:80", true},       // домашний роутер
		{"172.16.0.1:443", true},       // частная сеть (в т.ч. Docker)
		{"169.254.169.254:80", true},   // метаданные облака (AWS, GCP, Yandex Cloud)
		{"0.0.0.0:80", true},           // «любой адрес»
		{"149.154.166.110:443", false}, // публичный адрес
		{"8.8.8.8:53", false},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			err := BlockInternalAddresses("tcp", tt.addr, nil)
			if blocked := errors.Is(err, ErrForbiddenAddress); blocked != tt.blocked {
				t.Errorf("blocked=%v, ожидали %v (err=%v)", blocked, tt.blocked, err)
			}
		})
	}
}

// Проверяем всю цепочку: клиент не должен даже подключиться к локальному серверу.
func TestClient_LocalServerRefused(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	_, err := NewClient(5 * time.Second).Do(req)

	if !errors.Is(err, ErrForbiddenAddress) {
		t.Errorf("ожидали отказ из-за локального адреса, получили %v", err)
	}
	if called {
		t.Error("запрос дошёл до локального сервера")
	}
}

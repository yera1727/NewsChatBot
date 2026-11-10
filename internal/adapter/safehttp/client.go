// Package safehttp — HTTP-клиент для скачивания адресов, которые присылают пользователи.
//
// Запрещает соединения с внутренней сетью (localhost, 10.x, 192.168.x, 169.254.x и т.п.).
// Иначе через бота можно было бы обращаться к сервисам, закрытым от интернета, —
// например, к нашей же базе на localhost:5433. Такая атака называется SSRF.
package safehttp

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// MaxBodySize — сколько байт максимум стоит читать из ответа. Защита от огромных
// (или бесконечных) ответов, которые съели бы всю память.
const MaxBodySize = 10 << 20 // 10 МБ

var ErrForbiddenAddress = errors.New("адреса внутренней сети запрещены")

// NewClient возвращает http.Client с защитой от SSRF.
//
// Проверка стоит в Dialer.Control: он вызывается для КАЖДОГО соединения уже после
// DNS-резолва, поэтому её не обойти ни редиректом, ни DNS-записью на 127.0.0.1.
func NewClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, Control: BlockInternalAddresses}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialer.DialContext
	transport.Proxy = nil // через прокси проверялся бы адрес прокси, а не сайта

	return &http.Client{Timeout: timeout, Transport: transport}
}

// BlockInternalAddresses запрещает соединения с локальными и внутренними IP.
// Сигнатура совпадает с net.Dialer.Control.
func BlockInternalAddresses(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return fmt.Errorf("%w: %s", ErrForbiddenAddress, host)
	}
	return nil
}

// AllowLocalForTests снимает защиту с клиента. Нужна только в тестах, где
// тестовый сервер (httptest) работает на 127.0.0.1.
func AllowLocalForTests(c *http.Client) {
	c.Transport.(*http.Transport).DialContext = (&net.Dialer{}).DialContext
}

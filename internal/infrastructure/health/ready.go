// Пакет health проверяет готовность доверенных внутренних сервисов без раскрытия адресов.
package health

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// HTTPReady обращается к заданному внутреннему маршруту готовности без прокси и перенаправлений.
type HTTPReady struct {
	url    string
	client *http.Client
}

func NewHTTPReady(baseURL string) *HTTPReady {
	return &HTTPReady{url: strings.TrimRight(baseURL, "/") + "/health/ready", client: &http.Client{
		Timeout:       1500 * time.Millisecond,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second},
	}}
}

func (h *HTTPReady) Ready(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.url, nil)
	if err != nil {
		return false
	}
	response, err := h.client.Do(req)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func (h *HTTPReady) Close() { h.client.CloseIdleConnections() }

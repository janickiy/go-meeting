package media

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/media"
)

// HTTPClient выполняет внутренние медиа-вызовы; не передаёт секрет через редиректы и не раскрывает ответ с SDP в ошибке.
// @params
//   - secret: секрет подписи или внутренней авторизации компонента.
//   - client: клиент внешнего сервиса или транспорта компонента.
type HTTPClient struct {
	secret string
	client *http.Client
}

// NewHTTPClient создаёт и связывает зависимости компонента HTTPClient, используемого в защищённом управлении медиа-комнатой.
//
// @args
//   - secret (string): секрет подписи или внутренней авторизации компонента.
//   - timeout (time.Duration): максимальное время ожидания операции.
//
// @return:
//   - результат 1 (*HTTPClient): созданный компонент с переданными зависимостями.
func NewHTTPClient(secret string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{secret: secret, client: &http.Client{Timeout: timeout,
		CheckRedirect:/* Вложенный обработчик выполняет выделенный шаг обработки в защищённом управлении медиа-комнатой, используя состояние окружающей функции.

		@args
		  - аргумент 1 (*http.Request): входящий HTTP-запрос.
		  - аргумент 2 ([]*http.Request): входящий HTTP-запрос.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{Proxy: nil, MaxIdleConns: 64, MaxIdleConnsPerHost: 32, IdleConnTimeout: 60 * time.Second, ResponseHeaderTimeout: timeout},
	}}
}

// Call выполняет защищённый внутренний HTTP-вызов выбранной операции медиа-воркера.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - action (string): действие управления, которое необходимо проверить или исполнить.
//   - command (domain.Command): внутренняя команда с типом операции и серверной идентичностью ресурса.
//
// @return:
//   - результат 1 (domain.Result): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *HTTPClient) Call(ctx context.Context, action string, command domain.Command) (domain.Result, error) {
	if config.ValidateMediaEndpoint(command.Route.Endpoint) != nil {
		return domain.Result{}, domain.ErrInvalid
	}
	switch action {
	case "join", "offer", "ready", "ice", "leave", "unpublish", "policy", "close":
	default:
		return domain.Result{}, domain.ErrInvalid
	}
	raw, err := json.Marshal(command)
	if err != nil || len(raw) > 98304 {
		return domain.Result{}, domain.ErrInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(command.Route.Endpoint, "/")+"/internal/media/"+action, bytes.NewReader(raw))
	if err != nil {
		return domain.Result{}, domain.ErrInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("X-Request-ID", command.RequestID)
	resp, err := c.client.Do(req)
	if err != nil {
		return domain.Result{}, apperrors.Wrap(domain.ErrUnavailable, err, domain.ErrorCode(domain.ErrUnavailable))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 262145))
	if err != nil {
		return domain.Result{}, apperrors.Wrap(domain.ErrUnavailable, err, domain.ErrorCode(domain.ErrUnavailable))
	}
	if len(body) > 262144 {
		return domain.Result{}, domain.ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var failure struct {
			Code  string `json:"code"`
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &failure)
		code := failure.Code
		// Older workers used the error field for the same machine code.
		if code == "" {
			code = failure.Error
		}
		return domain.Result{}, domain.ErrorFromCode(code)
	}
	var result domain.Result
	if len(body) > 0 && json.Unmarshal(body, &result) != nil {
		return domain.Result{}, domain.ErrUnavailable
	}
	return result, nil
}

// Close закрывает принадлежащие компоненту ресурсы и завершает связанный жизненный цикл.
func (c *HTTPClient) Close() { c.client.CloseIdleConnections() }

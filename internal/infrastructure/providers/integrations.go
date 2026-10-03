package providers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	d "github.com/janickiy/go-recorder/internal/domain/integrations"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
)

// AdapterConfig задаёт режим доставки и доверенный endpoint серверного gateway; пользовательские URL не принимаются.
type AdapterConfig struct {
	Mode      string
	Endpoint  string
	Secret    string
	Timeout   time.Duration
	AllowHTTP bool
}

// IntegrationConfig объединяет независимые конфигурации каналов и OAuth внешнего календаря.
type IntegrationConfig struct {
	Email, Push, Calendar AdapterConfig
	OAuth                 OAuthConfig
	MockConnectAllowed    bool
}

// NewIntegrations создаёт заменяемые mock/noop/HTTP адаптеры, валидируя конфигурацию до запуска worker.
// @args cfg — серверные настройки каналов, разрешённых сетевых схем и OAuth.
// @return: набор провайдеров либо ошибка небезопасной конфигурации.
func NewIntegrations(cfg IntegrationConfig) (d.Providers, error) {
	e, err := newGateway(cfg.Email)
	if err != nil {
		return d.Providers{}, err
	}
	p, err := newGateway(cfg.Push)
	if err != nil {
		return d.Providers{}, err
	}
	c, err := newGateway(cfg.Calendar)
	if err != nil {
		return d.Providers{}, err
	}
	result := d.Providers{Email: &EmailAdapter{e}, Push: &PushAdapter{p}, Calendar: &CalendarAdapter{gateway: c}, Capabilities: d.Capabilities{Email: e.mode, Push: p.mode, Calendar: c.mode, MockConnectAllowed: cfg.MockConnectAllowed && c.mode == "mock"}}
	if cfg.OAuth.ClientID != "" {
		o, err := NewOAuth(cfg.OAuth)
		if err != nil {
			return result, err
		}
		result.OAuth = o
		result.Capabilities.CalendarOAuthConfigured = true
	}
	return result, nil
}

// gateway выполняет ограниченный JSON HTTP вызов или детерминированную имитацию без сетевого доступа.
type gateway struct {
	mode     string
	endpoint string
	secret   string
	client   *http.Client
}

// newGateway проверяет фиксированный адрес и запрещает перенаправление, способное раскрыть секрет провайдера.
// @args cfg — режим, endpoint, секрет, timeout и явное разрешение HTTP для тестового окружения.
// @return: настроенный gateway либо ошибка конфигурации.
func newGateway(cfg AdapterConfig) (*gateway, error) {
	mode := cfg.Mode
	if mode == "" {
		mode = "noop"
	}
	if mode != "noop" && mode != "mock" && mode != "http" {
		return nil, errors.New("invalid integration provider mode")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.Timeout > 5*time.Minute {
		return nil, errors.New("integration provider timeout exceeds five minutes")
	}
	g := &gateway{mode: mode, secret: cfg.Secret, endpoint: strings.TrimRight(cfg.Endpoint, "/"), client: &http.Client{Timeout: cfg.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	if mode == "http" {
		u, err := url.Parse(cfg.Endpoint)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || (u.Scheme != "https" && !(cfg.AllowHTTP && u.Scheme == "http")) {
			return nil, errors.New("integration endpoint must be a trusted HTTPS URL")
		}
		if cfg.Secret == "" {
			return nil, errors.New("HTTP integration provider secret is required")
		}
	}
	return g, nil
}

// call отправляет только заданную операцию с ключом идемпотентности; ответ и тело ошибки ограничены.
// @args ctx — отмена; operation — доверенная операция; key — стабильный ключ; input/output — JSON запрос и результат.
// @return: классифицированная ошибка, не включающая response body или credentials.
func (g *gateway) call(ctx context.Context, operation, key string, input, output any) error {
	if g.mode == "noop" {
		return jobs.ErrSkip
	}
	if g.mode == "mock" {
		return nil
	}
	body, err := json.Marshal(input)
	if err != nil {
		return jobs.Error{Code: "provider_input", Retryable: false}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint+"/"+operation, bytes.NewReader(body))
	if err != nil {
		return jobs.Error{Code: "provider_request"}
	}
	req.Header.Set("Authorization", "Bearer "+g.secret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	res, err := g.client.Do(req)
	if err != nil {
		return jobs.Error{Code: "provider_unavailable", Retryable: true}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20+1))
	if err != nil {
		return jobs.Error{Code: "provider_response", Retryable: true}
	}
	if len(raw) > 1<<20 {
		return jobs.Error{Code: "provider_response_too_large"}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		retry := res.StatusCode == 429 || res.StatusCode == 408 || res.StatusCode >= 500
		after := time.Duration(0)
		if seconds, err := time.ParseDuration(res.Header.Get("Retry-After") + "s"); err == nil && seconds > 0 && seconds <= time.Hour {
			after = seconds
		}
		code := "provider_http"
		if res.StatusCode == 429 {
			code = "provider_rate_limited"
		}
		return jobs.Error{Code: code, Retryable: retry, RetryAfter: after}
	}
	if output != nil && json.Unmarshal(raw, output) != nil {
		return jobs.Error{Code: "provider_invalid_response"}
	}
	return nil
}

// EmailAdapter реализует EmailProvider через серверный шлюз или явно обозначенные подставной и отключённый режимы.
type EmailAdapter struct{ gateway *gateway }

// Send доставляет письмо с идемпотентностью на стороне провайдера.
// @args ctx — отмена; message — минимальное письмо с экранированием и ключ доставки.
// @return: ошибка провайдера либо ErrSkip при noop.
func (a *EmailAdapter) Send(ctx context.Context, message d.EmailMessage) error {
	return a.gateway.call(ctx, "email/send", message.IdempotencyKey, message, nil)
}

// PushAdapter реализует безопасную доставку push через заменяемый gateway.
type PushAdapter struct{ gateway *gateway }

// Send отправляет push, не передавая полный transcript или OAuth-секреты.
// @args ctx — отмена; message — токен устройства, краткий текст и стабильный ключ.
// @return: ошибка либо ErrSkip при отключённом канале.
func (a *PushAdapter) Send(ctx context.Context, message d.PushMessage) error {
	return a.gateway.call(ctx, "push/send", message.IdempotencyKey, message, nil)
}

// CalendarAdapter сохраняет постоянный ID внешнего события и выполняет операции его жизненного цикла через шлюз.
type CalendarAdapter struct{ gateway *gateway }

// calendarInput сериализует credentials только в защищённый запрос доверенного gateway, не в API/logs.
type calendarInput struct {
	Event       d.CalendarEvent `json:"event"`
	AccessToken string          `json:"accessToken"`
}

// invoke выполняет операцию календаря; подставной режим возвращает одинаковый детерминированный ID при повторном создании.
// @args ctx — отмена; operation — фиксированная операция; credentials — серверный токен; event — vendor-neutral event.
// @return: безопасный event либо классифицированная ошибка.
func (a *CalendarAdapter) invoke(ctx context.Context, operation string, credentials d.CalendarCredentials, event d.CalendarEvent) (d.CalendarEvent, error) {
	if a.gateway.mode == "mock" {
		if event.ID == "" {
			sum := sha256.Sum256([]byte(event.IdempotencyKey))
			event.ID = "mock-" + hex.EncodeToString(sum[:12])
		}
		return event, nil
	}
	var out d.CalendarEvent
	err := a.gateway.call(ctx, "calendar/"+operation, event.IdempotencyKey+":"+operation, calendarInput{event, credentials.AccessToken}, &out)
	if err != nil {
		return out, err
	}
	if out.ID == "" || len(out.ID) > 512 || len(out.CalendarID) > 512 {
		return out, jobs.Error{Code: "calendar_missing_event_id"}
	}
	return out, nil
}

// CreateEvent создаёт событие, требуя от адаптера сохранения ключа идемпотентности.
// @args ctx — отмена; credentials — OAuth разрешение; event — расписание и стабильный ключ.
// @return: сохранённый внешний event либо ошибка.
func (a *CalendarAdapter) CreateEvent(ctx context.Context, c d.CalendarCredentials, e d.CalendarEvent) (d.CalendarEvent, error) {
	return a.invoke(ctx, "create", c, e)
}

// UpdateEvent обновляет ранее сохранённый внешний event, не создавая второй.
// @args ctx — отмена; c — OAuth разрешение; e — существующий id и новое расписание.
// @return: актуальный event либо ошибка.
func (a *CalendarAdapter) UpdateEvent(ctx context.Context, c d.CalendarCredentials, e d.CalendarEvent) (d.CalendarEvent, error) {
	return a.invoke(ctx, "update", c, e)
}

// CancelEvent отменяет событие с тем же ключом идемпотентности.
// @args ctx — отмена; c — OAuth разрешение; e — существующий event.
// @return: ошибка отмены либо nil.
func (a *CalendarAdapter) CancelEvent(ctx context.Context, c d.CalendarCredentials, e d.CalendarEvent) error {
	if a.gateway.mode == "mock" {
		return nil
	}
	return a.gateway.call(ctx, "calendar/cancel", e.IdempotencyKey+":cancel", calendarInput{e, c.AccessToken}, nil)
}

// GetEvent читает событие для диагностики или восстановления соответствия внешнего ресурса.
// @args ctx — отмена; c — OAuth разрешение; e — внешний идентификатор.
// @return: актуальный event либо ошибка.
func (a *CalendarAdapter) GetEvent(ctx context.Context, c d.CalendarCredentials, e d.CalendarEvent) (d.CalendarEvent, error) {
	return a.invoke(ctx, "get", c, e)
}

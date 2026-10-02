package providers

import (
	"context"
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

// OAuthConfig задаёт только доверенные серверные endpoints и минимальные scopes конкретного календарного адаптера.
type OAuthConfig struct {
	AuthorizationURL, TokenURL, RevokeURL, ClientID, ClientSecret, RedirectURL string
	Scopes                                                                     []string
	Timeout                                                                    time.Duration
	AllowHTTP                                                                  bool
}

// OAuth реализует authorization-code + S256 PKCE, refresh и revoke без vendor SDK.
type OAuth struct {
	cfg    OAuthConfig
	client *http.Client
}

// NewOAuth валидирует независимую конфигурацию OAuth и запрещает redirects с секретами.
// @args cfg — фиксированные endpoints, credentials и redirect URI.
// @return: адаптер либо ошибка небезопасной конфигурации.
func NewOAuth(cfg OAuthConfig) (*OAuth, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" || len(cfg.Scopes) == 0 {
		return nil, errors.New("calendar OAuth client credentials and minimal scopes are required")
	}
	for _, value := range []string{cfg.AuthorizationURL, cfg.TokenURL, cfg.RevokeURL, cfg.RedirectURL} {
		u, err := url.Parse(value)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && !(cfg.AllowHTTP && u.Scheme == "http")) {
			return nil, errors.New("calendar OAuth endpoints must be trusted HTTPS URLs")
		}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.Timeout > 5*time.Minute {
		return nil, errors.New("OAuth timeout exceeds five minutes")
	}
	return &OAuth{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// AuthorizeURL строит authorization запрос с одноразовым state и S256 PKCE challenge.
// @args state — случайный непрозрачный state; challenge — SHA-256 verifier в base64url.
// @return: URL redirect к настроенному серверу авторизации либо ошибка.
func (o *OAuth) AuthorizeURL(state, challenge string) (string, error) {
	u, err := url.Parse(o.cfg.AuthorizationURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_id", o.cfg.ClientID)
	q.Set("redirect_uri", o.cfg.RedirectURL)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(o.cfg.Scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// tokenResponse ограничивает чтение OAuth JSON полями, необходимыми календарю.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

// request выполняет фиксированный OAuth endpoint и не включает response body в ошибки.
// @args ctx — отмена; endpoint — доверенный URL; form — OAuth grant/revoke параметры; response — токены либо nil.
// @return: классифицированная безопасная ошибка.
func (o *OAuth) request(ctx context.Context, endpoint string, form url.Values, response *tokenResponse) error {
	form.Set("client_id", o.cfg.ClientID)
	form.Set("client_secret", o.cfg.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return jobs.Error{Code: "oauth_request"}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := o.client.Do(req)
	if err != nil {
		return jobs.Error{Code: "oauth_unavailable", Retryable: true}
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 64*1024+1))
	if err != nil {
		return jobs.Error{Code: "oauth_response", Retryable: true}
	}
	if len(b) > 64*1024 {
		return jobs.Error{Code: "oauth_response_size"}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		code := "oauth_http"
		if res.StatusCode == 429 {
			code = "oauth_rate_limited"
		}
		return jobs.Error{Code: code, Retryable: res.StatusCode == 429 || res.StatusCode >= 500}
	}
	if response != nil && (json.Unmarshal(b, response) != nil || response.AccessToken == "" || !strings.EqualFold(response.TokenType, "bearer") || response.ExpiresIn < 0 || response.ExpiresIn > 366*24*3600) {
		return jobs.Error{Code: "oauth_invalid_token"}
	}
	return nil
}

// credentials преобразует OAuth JSON в серверные credentials, не выдаваемые клиенту.
// @args r — валидированный OAuth response.
// @return: tokens, scopes и однозначное UTC expiry.
func credentials(r tokenResponse) d.CalendarCredentials {
	var at *time.Time
	if r.ExpiresIn > 0 {
		v := time.Now().UTC().Add(time.Duration(r.ExpiresIn) * time.Second)
		at = &v
	}
	return d.CalendarCredentials{AccessToken: r.AccessToken, RefreshToken: r.RefreshToken, ExpiresAt: at, Scopes: r.Scope}
}

// Exchange меняет одноразовый authorization code с PKCE verifier на серверные токены.
// @args ctx — отмена; code — authorization code; verifier — секрет S256 verifier.
// @return: credentials либо безопасная ошибка.
func (o *OAuth) Exchange(ctx context.Context, code, verifier string) (d.CalendarCredentials, error) {
	var r tokenResponse
	err := o.request(ctx, o.cfg.TokenURL, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}, "redirect_uri": {o.cfg.RedirectURL}}, &r)
	return credentials(r), err
}

// Refresh обновляет истёкший access token, сохраняя старый refresh token при отсутствии rotation.
// @args ctx — отмена; token — зашифрованный на диске refresh token после серверной расшифровки.
// @return: новые credentials либо ошибка.
func (o *OAuth) Refresh(ctx context.Context, token string) (d.CalendarCredentials, error) {
	var r tokenResponse
	err := o.request(ctx, o.cfg.TokenURL, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {token}}, &r)
	if r.RefreshToken == "" {
		r.RefreshToken = token
	}
	return credentials(r), err
}

// Revoke отзывает provider token; локальное отключение остаётся обязательным даже при сетевом сбое.
// @args ctx — отмена; token — access/refresh token владельца подключения.
// @return: безопасная ошибка внешнего отзыва.
func (o *OAuth) Revoke(ctx context.Context, token string) error {
	return o.request(ctx, o.cfg.RevokeURL, url.Values{"token": {token}}, nil)
}

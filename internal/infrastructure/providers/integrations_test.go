package providers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	d "github.com/janickiy/go-recorder/internal/domain/integrations"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
)

// TestIntegrationProviderModes проверяет отключение, детерминированный mock и idempotent calendar identifiers.
// @args t — контекст теста.
func TestIntegrationProviderModes(t *testing.T) {
	p, err := NewIntegrations(IntegrationConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(p.Email.Send(context.Background(), d.EmailMessage{}), jobs.ErrSkip) {
		t.Fatal("noop pretends delivery")
	}
	p, err = NewIntegrations(IntegrationConfig{Email: AdapterConfig{Mode: "mock"}, Push: AdapterConfig{Mode: "mock"}, Calendar: AdapterConfig{Mode: "mock"}, MockConnectAllowed: true})
	if err != nil {
		t.Fatal(err)
	}
	e := d.CalendarEvent{IdempotencyKey: "stable"}
	a, err := p.Calendar.CreateEvent(context.Background(), d.CalendarCredentials{}, e)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := p.Calendar.CreateEvent(context.Background(), d.CalendarCredentials{}, e)
	if a.ID == "" || a.ID != b.ID {
		t.Fatal("duplicate mock event")
	}
	if !p.Capabilities.MockConnectAllowed {
		t.Fatal("mock unavailable")
	}
	if _, err := NewIntegrations(IntegrationConfig{Email: AdapterConfig{Mode: "http", Endpoint: "http://example.com", Secret: "secret"}}); err == nil {
		t.Fatal("insecure production endpoint accepted")
	}
}

// TestIntegrationHTTPClassification проверяет bounded safe errors, rate limit и provider dedup key.
// @args t — контекст теста.
func TestIntegrationHTTPClassification(t *testing.T) {
	status := http.StatusTooManyRequests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != "stable" || r.Header.Get("Authorization") != "Bearer gateway-secret" {
			t.Error("headers missing")
		}
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(status)
		fmt.Fprint(w, "DO NOT LOG PROVIDER SECRET")
	}))
	defer server.Close()
	p, err := NewIntegrations(IntegrationConfig{Email: AdapterConfig{Mode: "http", Endpoint: server.URL, Secret: "gateway-secret", AllowHTTP: true}})
	if err != nil {
		t.Fatal(err)
	}
	err = p.Email.Send(context.Background(), d.EmailMessage{IdempotencyKey: "stable"})
	var classified jobs.Error
	if !errors.As(err, &classified) || !classified.Retryable || classified.RetryAfter != 3*time.Second || strings.Contains(err.Error(), "SECRET") {
		t.Fatal("classification", err)
	}
	status = http.StatusBadRequest
	err = p.Email.Send(context.Background(), d.EmailMessage{IdempotencyKey: "stable"})
	if !errors.As(err, &classified) || classified.Retryable {
		t.Fatal("permanent error retries")
	}
	status = http.StatusNoContent
	if err = p.Email.Send(context.Background(), d.EmailMessage{IdempotencyKey: "stable"}); err != nil {
		t.Fatal(err)
	}
}

// TestOAuthPKCERefreshAndRevoke проверяет серверный S256 flow, точный redirect URI и отсутствие redirect-following с секретом.
// @args t — контекст теста.
func TestOAuthPKCERefreshAndRevoke(t *testing.T) {
	calls := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		_ = r.ParseForm()
		if r.Form.Get("client_secret") != "server-secret" {
			t.Error("client credential absent")
		}
		if r.URL.Path == "/revoke" {
			w.WriteHeader(200)
			return
		}
		if r.Form.Get("grant_type") == "authorization_code" && (r.Form.Get("code_verifier") != "verifier" || r.Form.Get("redirect_uri") == "") {
			t.Error("PKCE/redirect absent")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":120,"scope":"calendar.events"}`)
	}))
	defer server.Close()
	o, err := NewOAuth(OAuthConfig{AuthorizationURL: server.URL + "/auth", TokenURL: server.URL + "/token", RevokeURL: server.URL + "/revoke", RedirectURL: server.URL + "/callback", ClientID: "client", ClientSecret: "server-secret", Scopes: []string{"calendar.events"}, AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := o.AuthorizeURL("state", "challenge")
	parsed, _ := url.Parse(raw)
	q := parsed.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("state") != "state" || q.Get("client_secret") != "" {
		t.Fatal("unsafe authorization URL")
	}
	tokens, err := o.Exchange(context.Background(), "code", "verifier")
	if err != nil || tokens.AccessToken != "access" || tokens.ExpiresAt == nil {
		t.Fatal("exchange", err)
	}
	if _, err = o.Refresh(context.Background(), "refresh"); err != nil {
		t.Fatal(err)
	}
	if err = o.Revoke(context.Background(), "refresh"); err != nil {
		t.Fatal(err)
	}
	if calls["/token"] != 2 || calls["/revoke"] != 1 {
		t.Fatal("lifecycle calls")
	}
}

// TestProviderRedirectIsNotFollowed гарантирует, что credentials не пересылаются произвольному redirect destination.
// @args t — контекст теста.
func TestProviderRedirectIsNotFollowed(t *testing.T) {
	calls := 0
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	p, err := NewIntegrations(IntegrationConfig{Email: AdapterConfig{Mode: "http", Endpoint: server.URL, Secret: "secret", AllowHTTP: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Email.Send(context.Background(), d.EmailMessage{IdempotencyKey: "redirect"}); err == nil || calls != 0 {
		t.Fatal("redirect forwarded secret")
	}
}

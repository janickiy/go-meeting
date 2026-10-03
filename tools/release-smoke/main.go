// Команда release-smoke проверяет конкретный выпуск через публичный HTTP-прокси.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// result содержит только технические результаты без токенов, паролей и пользовательских ответов.
// @params: Environment — цель; Version/Commit — ожидаемый выпуск; Checks — успешные проверки;
// DurationMS — фактическое время; FixtureID — отменённая тестовая встреча в изолированном окружении.
type result struct {
	Environment string   `json:"environment"`
	Version     string   `json:"version"`
	Commit      string   `json:"commit"`
	Checks      []string `json:"checks"`
	DurationMS  int64    `json:"durationMs"`
	FixtureID   string   `json:"fixtureId,omitempty"`
}

// probe хранит параметры одного ограниченного запуска и не журналирует HTTP-содержимое.
// @params: base — проверенный адрес; client — HTTP-клиент; token — краткоживущая авторизация; ctx — срок проверки.
type probe struct {
	base   string
	client *http.Client
	token  string
	ctx    context.Context
	tls    *tls.Config
}

// main разбирает обязательную цель и выводит машиночитаемый результат только после успешного завершения.
func main() {
	environment := flag.String("environment", "", "local, staging or production")
	base := flag.String("base-url", "", "public frontend origin")
	version := flag.String("version", "", "expected immutable release version")
	commit := flag.String("commit", "", "expected commit SHA")
	exercise := flag.Bool("exercise", false, "create and cancel a dedicated local/staging conference")
	flag.Parse()
	out, err := run(*environment, *base, *version, *commit, *exercise)
	if err != nil {
		fmt.Fprintln(os.Stderr, "release smoke failed:", err)
		os.Exit(1)
	}
	if json.NewEncoder(os.Stdout).Encode(out) != nil {
		os.Exit(1)
	}
}

// validateTarget запрещает неявное окружение, URL с учётными данными и мутации production.
// @args environment — выбранная среда; raw — публичный origin; exercise — создание тестовой встречи.
// @return канонический URL или причину отказа без его содержимого.
func validateTarget(environment, raw string, exercise bool) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("a plain frontend origin is required")
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	switch environment {
	case "local":
		if !local || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, errors.New("local smoke requires a loopback origin")
		}
	case "staging", "production":
		if local || u.Scheme != "https" {
			return nil, errors.New("staging and production require non-loopback HTTPS with certificate verification")
		}
		if environment == "production" && exercise {
			return nil, errors.New("production smoke is read-only")
		}
	default:
		return nil, errors.New("an explicit target environment is required")
	}
	u.Path = ""
	return u, nil
}

// request выполняет один запрос с пределом размера ответа и без перенаправления секретов.
// @args method/path — фиксированная операция; body — параметры; status — ожидаемый код; target — безопасная модель ответа.
// @return категорию отказа без тела ответа или значения авторизации.
func (p *probe) request(method, path string, body any, status int, target any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return errors.New("request encoding failed")
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(p.ctx, method, p.base+path, reader)
	if err != nil {
		return errors.New("invalid request")
	}
	request.Header.Set("Content-Type", "application/json")
	if p.token != "" && strings.HasPrefix(path, "/api/") {
		request.Header.Set("Authorization", "Bearer "+p.token)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return errors.New("HTTP connection or TLS verification failed")
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		return fmt.Errorf("unexpected HTTP status %d, expected %d", response.StatusCode, status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return errors.New("response read failed or exceeded size limit")
	}
	if target != nil && json.Unmarshal(data, target) != nil {
		return errors.New("invalid JSON response")
	}
	return nil
}

// run проверяет одинаковую версию API и интерфейса, вход и минимальные права тестового пользователя.
// @args environment/base — цель; version/commit — ожидаемый артефакт; exercise — локальная проверка REST/WS с очисткой участия.
// @return результаты только пройденных проверок либо ошибку, блокирующую дальнейшее продвижение.
func run(environment, base, version, commit string, exercise bool) (result, error) {
	started := time.Now()
	out := result{Environment: environment, Version: version, Commit: commit, Checks: []string{}}
	u, err := validateTarget(environment, base, exercise)
	if err != nil {
		return out, err
	}
	if version == "" || version == "dev" || len(commit) != 40 {
		return out, errors.New("expected release version and full commit SHA are required")
	}
	if os.Getenv("SMOKE_EMAIL") == "" || os.Getenv("SMOKE_PASSWORD") == "" {
		return out, errors.New("external dedicated SMOKE_EMAIL and SMOKE_PASSWORD are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	tlsConfig, err := smokeTLS(os.Getenv("SMOKE_CA_FILE"))
	if err != nil {
		return out, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	defer transport.CloseIdleConnections()
	p := &probe{base: u.String(), ctx: ctx, tls: tlsConfig, client: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	for _, path := range []string{"/version", "/version.json"} {
		var info struct{ Version, Commit string }
		if err := p.request("GET", path, nil, 200, &info); err != nil {
			return out, fmt.Errorf("version check: %w", err)
		}
		if info.Version != version || info.Commit != commit {
			return out, errors.New("API/frontend version differs from release manifest")
		}
	}
	out.Checks = append(out.Checks, "api-and-frontend-version")
	if err := p.request("GET", "/", nil, 200, nil); err != nil {
		return out, fmt.Errorf("frontend: %w", err)
	}
	out.Checks = append(out.Checks, "frontend")
	var auth struct {
		Token string `json:"accessToken"`
	}
	if err := p.request("POST", "/api/v1/auth/login", map[string]string{"email": os.Getenv("SMOKE_EMAIL"), "password": os.Getenv("SMOKE_PASSWORD")}, 200, &auth); err != nil || auth.Token == "" {
		return out, errors.New("dedicated account login failed")
	}
	p.token = auth.Token
	for _, path := range []string{"/api/v1/auth/me", "/api/v1/capabilities", "/api/v1/webrtc/config"} {
		if err := p.request("GET", path, nil, 200, nil); err != nil {
			return out, fmt.Errorf("authenticated API: %w", err)
		}
	}
	if err := p.request("GET", "/api/v1/admin/summary", nil, 403, nil); err != nil {
		return out, errors.New("smoke requires a dedicated non-admin account")
	}
	out.Checks = append(out.Checks, "auth", "capabilities", "ice-configuration", "admin-denied")
	if exercise {
		if err := p.conference(&out); err != nil {
			return out, err
		}
	}
	out.DurationMS = time.Since(started).Milliseconds()
	return out, nil
}

// conference создаёт отдельную тестовую встречу и проверяет настоящий HTTP upgrade и снимок WS.
// @args out — результат с идентификатором тестовых данных для адресной последующей очистки.
// @return ошибку сценария или обязательного закрытия встречи; созданные записи остаются только в тестовом окружении.
func (p *probe) conference(out *result) (resultErr error) {
	var created struct{ Item struct{ ID string } }
	if err := p.request("POST", "/api/v1/conferences", map[string]string{"title": "release-smoke-" + uuid.NewString()}, 201, &created); err != nil {
		return fmt.Errorf("create conference: %w", err)
	}
	if _, err := uuid.Parse(created.Item.ID); err != nil {
		return errors.New("invalid conference identity")
	}
	out.FixtureID = created.Item.ID
	path := "/api/v1/conferences/" + created.Item.ID
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cleaner := *p
		cleaner.ctx = cleanup
		leaveErr := cleaner.request("POST", path+"/leave", nil, 200, nil)
		cancelErr := cleaner.request("POST", path+"/cancel", nil, 200, nil)
		if leaveErr != nil || cancelErr != nil {
			resultErr = errors.New("conference cleanup failed; inspect the dedicated test account")
		}
	}()
	if err := p.request("POST", path+"/join", nil, 200, nil); err != nil {
		return fmt.Errorf("join conference: %w", err)
	}
	wsURL, _ := url.Parse(p.base + path + "/ws")
	wsURL.Scheme = "wss"
	if strings.HasPrefix(p.base, "http://") {
		wsURL.Scheme = "ws"
	}
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, TLSClientConfig: p.tls}
	connection, response, err := dialer.DialContext(p.ctx, wsURL.String(), http.Header{"Authorization": []string{"Bearer " + p.token}})
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil {
		return errors.New("WebSocket authentication or upgrade failed")
	}
	defer connection.Close()
	connection.SetReadLimit(1 << 20)
	connection.SetReadDeadline(time.Now().Add(10 * time.Second))
	var event struct{ Type string }
	if connection.ReadJSON(&event) != nil || event.Type != "conference.state" {
		return errors.New("WebSocket conference snapshot failed")
	}
	out.Checks = append(out.Checks, "conference-create-join", "websocket-snapshot")
	return nil
}

// smokeTLS добавляет доверие к явно указанному сертификату только внутри одного запуска проверки.
// @args caFile — необязательный PEM-файл локального или корпоративного центра сертификации.
// @return настройки TLS с обязательной проверкой имени узла и цепочки либо безопасную ошибку.
func smokeTLS(caFile string) (*tls.Config, error) {
	configuration := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile == "" {
		return configuration, nil
	}
	info, err := os.Stat(caFile)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("SMOKE_CA_FILE must be a readable bounded PEM file")
	}
	data, err := os.ReadFile(caFile)
	if err != nil {
		return nil, errors.New("SMOKE_CA_FILE could not be read")
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(data) {
		return nil, errors.New("SMOKE_CA_FILE contains no valid certificates")
	}
	configuration.RootCAs = roots
	return configuration, nil
}

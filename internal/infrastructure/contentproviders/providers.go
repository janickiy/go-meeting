// Package contentproviders содержит заменяемые noop/mock/HTTP адаптеры STT и AI.
// Generic HTTP contract документируется отдельно; vendor SDK не проникают в domain.
package contentproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	domain "github.com/janickiy/go-recorder/internal/domain/content"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// provider содержит только серверные настройки; token не возвращается и не логируется.
type provider struct {
	mode, endpoint, token, model string
	client                       *http.Client
}

// newProvider проверяет доверенный endpoint и отключает redirects с credentials.
// @args mode — noop/mock/http; endpoint/token — серверные secrets/config;
// model — техническое имя; timeout — конечный deadline внешнего вызова.
// @return адаптер либо bootstrap error без раскрытия значения credential.
func newProvider(mode, endpoint, token, model string, timeout time.Duration) (*provider, error) {
	if mode != "noop" && mode != "mock" && mode != "http" {
		return nil, fmt.Errorf("invalid content provider mode")
	}
	if timeout <= 0 || timeout > time.Hour {
		return nil, fmt.Errorf("invalid content provider timeout")
	}
	if mode == "http" {
		u, e := url.Parse(endpoint)
		if e != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || token == "" {
			return nil, fmt.Errorf("HTTP provider requires trusted URL and credential")
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 8
	transport.ResponseHeaderTimeout = min(timeout, 30*time.Second)
	return &provider{mode: mode, endpoint: endpoint, token: token, model: model, client: &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// NewTranscriptionProvider создаёт STT adapter без network call при bootstrap.
// @args mode — явный режим; endpoint/token — доверенная настройка; timeout — finite limit.
// @return заменяемый TranscriptionProvider либо config error.
func NewTranscriptionProvider(mode, endpoint, token string, timeout time.Duration) (domain.TranscriptionProvider, error) {
	p, e := newProvider(mode, endpoint, token, "", timeout)
	if e != nil {
		return nil, e
	}
	return &transcription{p}, e
}

// NewAIProvider создаёт AI adapter; mock означает демонстрацию, а не настоящий AI.
// @args mode/endpoint/token/model — серверная настройка; timeout — finite limit.
// @return заменяемый AIProvider либо config error.
func NewAIProvider(mode, endpoint, token, model string, timeout time.Duration) (domain.AIProvider, error) {
	p, e := newProvider(mode, endpoint, token, model, timeout)
	if e != nil {
		return nil, e
	}
	return &intelligence{p}, e
}

// transcription реализует STT generic contract; mock выдаёт очевидно тестовый текст.
type transcription struct{ *provider }

// Name возвращает режим для честной маркировки mock в UI.
// @return noop, mock или http.
func (p *transcription) Name() string { return p.mode }

// Transcribe передаёт ограниченное WAV как streaming multipart без видео/URL.
// @args ctx — deadline; request — ограниченный reader и stable dedup key.
// @return непроверенный STT output; schema/time validation выполняется usecase.
func (p *transcription) Transcribe(ctx context.Context, request domain.TranscriptionRequest) (domain.TranscriptionResult, error) {
	if p.mode == "noop" {
		return domain.TranscriptionResult{}, jobs.ErrSkip
	}
	if p.mode == "mock" {
		return domain.TranscriptionResult{Language: "ru", Segments: []domain.Segment{{StartMS: 0, EndMS: min(int64(request.DurationSec)*1000, 5000), Text: "ТЕСТОВАЯ РАСШИФРОВКА: обсуждение проекта и следующих шагов. Это демонстрационный текст, не результат распознавания аудио."}}}, nil
	}
	if request.Audio == nil || request.Size <= 0 || request.Size > 2<<30 {
		return domain.TranscriptionResult{}, &jobs.Error{Code: "invalid_audio", Retryable: false}
	}
	var header bytes.Buffer
	writer := multipart.NewWriter(&header)
	if e := writer.WriteField("language", request.Language); e != nil {
		return domain.TranscriptionResult{}, e
	}
	if _, e := writer.CreateFormFile("audio", "meeting.wav"); e != nil {
		return domain.TranscriptionResult{}, e
	}
	prefix := append([]byte(nil), header.Bytes()...)
	header.Reset()
	if e := writer.Close(); e != nil {
		return domain.TranscriptionResult{}, e
	}
	suffix := append([]byte(nil), header.Bytes()...)
	body := io.MultiReader(bytes.NewReader(prefix), io.LimitReader(request.Audio, request.Size), bytes.NewReader(suffix))
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, body)
	if e != nil {
		return domain.TranscriptionResult{}, &jobs.Error{Code: "provider_request", Retryable: false}
	}
	req.ContentLength = int64(len(prefix)) + request.Size + int64(len(suffix))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	data, e := p.call(req, request.IdempotencyKey, 10<<20)
	if e != nil {
		return domain.TranscriptionResult{}, e
	}
	var out domain.TranscriptionResult
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil {
		return out, &jobs.Error{Code: "invalid_transcript", Retryable: false}
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return out, &jobs.Error{Code: "invalid_transcript", Retryable: false}
	}
	return out, nil
}

// intelligence реализует JSON-only AI без tools/function/network capabilities.
type intelligence struct{ *provider }

// Name возвращает публичное имя режима без endpoint/credential.
// @return noop, mock или http.
func (p *intelligence) Name() string { return p.mode }

// Model возвращает воспроизводимое имя модели для metadata.
// @return configured name или явное deterministic mock обозначение.
func (p *intelligence) Model() string {
	if p.mode == "mock" {
		return "deterministic-demo"
	}
	return p.model
}

// Summarize отправляет immutable instructions отдельно от UNTRUSTED JSON.
// @args ctx — deadline; request — bounded input и schema/prompt versions.
// @return raw JSON output, обязательно проверяемый usecase перед сохранением.
func (p *intelligence) Summarize(ctx context.Context, request domain.AIRequest) (json.RawMessage, error) {
	if p.mode == "noop" {
		return nil, jobs.ErrSkip
	}
	if p.mode == "mock" {
		return json.Marshal(domain.SummaryOutput{Summary: "ТЕСТОВАЯ СВОДКА: обсуждение проекта и следующих шагов. Демонстрация, не реальный AI-анализ.", KeyPoints: []string{"Тестовые данные; провайдер mock"}, ActionItems: []domain.ActionItem{}, Topics: []string{"Демонстрация"}})
	}
	if len(request.InputJSON) > 1<<20 || len(request.Instructions) > 20000 {
		return nil, &jobs.Error{Code: "ai_input_limit", Retryable: false}
	}
	body, e := json.Marshal(map[string]any{"model": p.model, "promptVersion": request.PromptVersion, "schemaVersion": request.SchemaVersion, "instructions": request.Instructions, "input": json.RawMessage(request.InputJSON), "merge": request.Merge, "responseFormat": "meeting-summary-v1", "tools": []any{}})
	if e != nil {
		return nil, &jobs.Error{Code: "invalid_ai_input", Retryable: false}
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if e != nil {
		return nil, &jobs.Error{Code: "provider_request", Retryable: false}
	}
	req.Header.Set("Content-Type", "application/json")
	return p.call(req, request.IdempotencyKey, 256<<10)
}

// call применяет finite timeout/response bounds и возвращает только safe error codes.
// @args request — server-owned HTTP request; key — stable idempotency;
// maximum — максимальный размер ответа, не сохраняемого в logs.
// @return ограниченные bytes или retry/permanent classification.
func (p *provider) call(request *http.Request, key string, maximum int64) ([]byte, error) {
	request.Header.Set("Authorization", "Bearer "+p.token)
	request.Header.Set("Idempotency-Key", key)
	response, e := p.client.Do(request)
	if e != nil {
		return nil, &jobs.Error{Code: "provider_unavailable", Retryable: true}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		retry := response.StatusCode == 429 || response.StatusCode == 408 || response.StatusCode >= 500
		code := "provider_rejected"
		if response.StatusCode == 429 {
			code = "provider_rate_limited"
		}
		return nil, &jobs.Error{Code: code, Retryable: retry, RetryAfter: retryAfter(response.Header.Get("Retry-After"))}
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, maximum+1))
	if e != nil {
		return nil, &jobs.Error{Code: "provider_unavailable", Retryable: true}
	}
	if int64(len(data)) > maximum {
		return nil, &jobs.Error{Code: "provider_response_limit", Retryable: false}
	}
	return data, nil
}

// retryAfter ограничивает Retry-After, не позволяя внешнему API заморозить job навсегда.
// @args value — HTTP header в seconds или HTTP-date.
// @return пауза от нуля до часа; неизвестное значение означает обычный backoff.
func retryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, e := strconv.Atoi(value); e == nil {
		return max(0, min(time.Duration(seconds)*time.Second, time.Hour))
	}
	if at, e := http.ParseTime(value); e == nil {
		return max(0, min(time.Until(at), time.Hour))
	}
	return 0
}

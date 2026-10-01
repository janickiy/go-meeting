package media

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/janickiy/go-recorder/internal/config"
	domain "github.com/janickiy/go-recorder/internal/domain/media"
)

// HTTPClient never follows redirects with its internal credential and never
// exposes upstream response text (which could contain SDP) as an error.
type HTTPClient struct {
	secret string
	client *http.Client
}

func NewHTTPClient(secret string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{secret: secret, client: &http.Client{Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{Proxy: nil, MaxIdleConns: 64, MaxIdleConnsPerHost: 32, IdleConnTimeout: 60 * time.Second, ResponseHeaderTimeout: timeout},
	}}
}

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
		return domain.Result{}, domain.ErrUnavailable
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 262145))
	if err != nil || len(body) > 262144 {
		return domain.Result{}, domain.ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var failure struct {
			Code  string `json:"code"`
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &failure)
		code := failure.Code
		if code == "" {
			code = failure.Error
		}
		for _, safe := range []error{domain.ErrInvalid, domain.ErrUnauthorized, domain.ErrOwnership, domain.ErrLimit, domain.ErrPeerNotFound, domain.ErrNegotiation, domain.ErrScreenConflict, domain.ErrPolicy} {
			if code == safe.Error() {
				return domain.Result{}, safe
			}
		}
		return domain.Result{}, domain.ErrUnavailable
	}
	var result domain.Result
	if len(body) > 0 && json.Unmarshal(body, &result) != nil {
		return domain.Result{}, domain.ErrUnavailable
	}
	return result, nil
}

func (c *HTTPClient) Close() { c.client.CloseIdleConnections() }

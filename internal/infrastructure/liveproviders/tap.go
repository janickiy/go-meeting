package liveproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"io"
	"net/http"
	"strings"
	"time"
)

// Registry разрешает только адрес текущего владельца комнаты из доверенного внутреннего реестра.
type Registry interface {
	GetOwner(context.Context, string) (media.Route, error)
}

// Tap открывает отдельный HTTP-поток без проксирования медиа через API и RabbitMQ.
type Tap struct {
	Registry Registry
	Secret   string
	Client   *http.Client
}

// NewTap создаёт внутренний клиент с ограниченным установлением соединения и без redirects.
// @args registry — реестр SFU; secret — независимый service credential.
// @return аудиоподписка, поток которой ограничен временем жизни контекста.
func NewTap(registry Registry, secret string) *Tap {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 5 * time.Second
	transport.MaxConnsPerHost = 32
	return &Tap{registry, secret, &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// Open получает владельца и открывает только назначение captions.
// @args ctx — отмена потока; cid,id — конференция и серверная сессия.
// @return закрываемое NDJSON тело либо безопасная ошибка без endpoint/секрета.
func (t *Tap) Open(ctx context.Context, cid, id string) (io.ReadCloser, error) {
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	route, err := t.Registry.GetOwner(op, cid)
	cancel()
	if err != nil || config.ValidateMediaEndpoint(route.Endpoint) != nil {
		return nil, media.ErrUnavailable
	}
	command := media.EgressRequest{Purpose: "captions", RequestID: uuid.NewString(), ConferenceID: cid, RecordingID: id, Route: route, SegmentDurationSec: 5}
	raw, _ := json.Marshal(command)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(route.Endpoint, "/")+"/internal/media/egress", bytes.NewReader(raw))
	if err != nil {
		return nil, media.ErrUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+t.Secret)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", command.RequestID)
	response, err := t.Client.Do(request)
	if err != nil {
		return nil, media.ErrUnavailable
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, media.ErrUnavailable
	}
	return response.Body, nil
}

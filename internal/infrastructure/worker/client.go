package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/janickiy/meet-space/internal/domain/records"
	"github.com/janickiy/meet-space/internal/operations"
)

const defaultTimeout = 10 * time.Minute

// Client объединяет настройки и соединения клиента соответствующего внешнего сервиса.
// @params
//   - baseURL: значение baseURL типа string, используемое согласно назначению этой операции.
//   - client: клиент внешнего сервиса или транспорта компонента.
type Client struct {
	baseURL string
	client  *http.Client
	secret  string
}

// SetSecret защищает внутренние команды recorder-worker. secret — отдельный
// серверный ключ; возвращает клиент для связывания при запуске, до запросов.
func (c *Client) SetSecret(secret string) *Client { c.secret = secret; return c }

// NewClient создает HTTP-клиент recorder-worker.
// @args
// - baseURL: внутренний URL воркера, например http://worker:8090.
// @return Client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: defaultTimeout},
	}
}

// StartRecord просит воркер подготовить приём WebRTC для записи.
// @args
// - ctx: контекст HTTP-запроса API.
// - recordID: UUID записи.
// - segmentDurationSec: длительность сегмента в секундах.
// @return ошибку внутреннего HTTP-вызова.
func (c *Client) StartRecord(ctx context.Context, recordID string, segmentDurationSec int) error {
	return c.sendCommand(ctx, recordID, "start", records.Command{
		Type:               "record.start",
		RecordID:           recordID,
		SegmentDurationSec: segmentDurationSec,
	})
}

// StopRecord просит worker остановить запись и выполнить финализацию.
// @args
// - ctx: контекст HTTP-запроса API.
// - recordID: UUID записи.
// - reason: причина остановки.
// @return ошибку внутреннего HTTP-вызова.
func (c *Client) StopRecord(ctx context.Context, recordID string, reason string) error {
	return c.sendCommand(ctx, recordID, "stop", records.Command{
		Type:     "record.stop",
		RecordID: recordID,
		Reason:   reason,
	})
}

// Offer отправляет SDP-предложение браузера воркеру и возвращает SDP-ответ.
// @args
// - ctx: контекст HTTP-запроса API.
// - recordID: UUID записи.
// - request: SDP-предложение браузера.
// @return SDP answer или ошибку signaling.
func (c *Client) Offer(ctx context.Context, recordID string, request records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error) {
	var response records.WebRTCAnswerResponse
	if err := c.postJSON(ctx, c.recordEndpoint(recordID, "webrtc/offer"), request, &response); err != nil {
		return records.WebRTCAnswerResponse{}, err
	}

	return response, nil
}

// sendCommand отправляет сериализованную команду управления записью.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - recordID (string): внешний UUID задачи записи.
//   - action (string): действие управления, которое необходимо проверить или исполнить.
//   - command (records.Command): внутренняя команда с типом операции и серверной идентичностью ресурса.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Client) sendCommand(ctx context.Context, recordID string, action string, command records.Command) error {
	return c.postJSON(ctx, c.recordEndpoint(recordID, action), command, nil)
}

// recordEndpoint строит адрес маршрута воркера для конкретной записи.
//
// @args
//   - recordID (string): внешний UUID задачи записи.
//   - action (string): действие управления, которое необходимо проверить или исполнить.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func (c *Client) recordEndpoint(recordID string, action string) string {
	return fmt.Sprintf("%s/records/%s/%s", c.baseURL, neturl.PathEscape(recordID), action)
}

// postJSON выполняет HTTP POST с JSON и обрабатывает результат внутреннего вызова.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - endpoint (string): адрес конечной точки вызываемого сервиса.
//   - payload (any): типизированная нагрузка события или ссылочные сведения уведомления.
//   - target (any): целевой объект, участник или состояние операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Client) postJSON(ctx context.Context, endpoint string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	if id := operations.ID(ctx); id != "" {
		req.Header.Set("X-Request-ID", id)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(responseBody))
		if message == "" {
			message = resp.Status
		}

		return errors.New(message)
	}
	if target == nil {
		return nil
	}
	if err := json.Unmarshal(responseBody, target); err != nil {
		return fmt.Errorf("decode worker response: %w", err)
	}

	return nil
}

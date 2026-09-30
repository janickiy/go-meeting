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

	"github.com/janickiy/go-recorder/internal/domain/records"
)

const defaultTimeout = 10 * time.Minute

// Client выполняет внутренние HTTP-запросы из API в recorder-worker.
type Client struct {
	baseURL string
	client  *http.Client
}

// NewClient создает HTTP-клиент recorder-worker.
// Параметры:
// - baseURL: внутренний URL worker-а, например http://worker:8090.
// Возвращает: Client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: defaultTimeout},
	}
}

// StartRecord просит worker подготовить WebRTC ingest для записи.
// Параметры:
// - ctx: контекст HTTP-запроса API.
// - recordID: UUID записи.
// - segmentDurationSec: длительность сегмента в секундах.
// Возвращает: ошибку внутреннего HTTP-вызова.
func (c *Client) StartRecord(ctx context.Context, recordID string, segmentDurationSec int) error {
	return c.sendCommand(ctx, recordID, "start", records.Command{
		Type:               "record.start",
		RecordID:           recordID,
		SegmentDurationSec: segmentDurationSec,
	})
}

// StopRecord просит worker остановить запись и выполнить финализацию.
// Параметры:
// - ctx: контекст HTTP-запроса API.
// - recordID: UUID записи.
// - reason: причина остановки.
// Возвращает: ошибку внутреннего HTTP-вызова.
func (c *Client) StopRecord(ctx context.Context, recordID string, reason string) error {
	return c.sendCommand(ctx, recordID, "stop", records.Command{
		Type:     "record.stop",
		RecordID: recordID,
		Reason:   reason,
	})
}

// Offer отправляет browser SDP offer во worker и возвращает SDP answer.
// Параметры:
// - ctx: контекст HTTP-запроса API.
// - recordID: UUID записи.
// - request: SDP offer браузера.
// Возвращает: SDP answer или ошибку signaling.
func (c *Client) Offer(ctx context.Context, recordID string, request records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error) {
	var response records.WebRTCAnswerResponse
	if err := c.postJSON(ctx, c.recordEndpoint(recordID, "webrtc/offer"), request, &response); err != nil {
		return records.WebRTCAnswerResponse{}, err
	}

	return response, nil
}

func (c *Client) sendCommand(ctx context.Context, recordID string, action string, command records.Command) error {
	return c.postJSON(ctx, c.recordEndpoint(recordID, action), command, nil)
}

func (c *Client) recordEndpoint(recordID string, action string) string {
	return fmt.Sprintf("%s/records/%s/%s", c.baseURL, neturl.PathEscape(recordID), action)
}

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

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
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

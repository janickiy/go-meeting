package recordsapp

import (
	"context"

	"git.svc-dev.net/board/go-recorder/internal/domain/records"
	workerinfra "git.svc-dev.net/board/go-recorder/internal/infrastructure/worker"
)

// Service описывает use-case, который нужен HTTP handler-у записей.
type Service interface {
	Start(ctx context.Context, request records.StartRequest) (records.StartResponse, error)
	Stop(ctx context.Context, request records.EndRequest) error
	List(ctx context.Context, limit int, offset int) ([]records.RecordCard, error)
	CountByConference(ctx context.Context, conferenceIDs []string, status string) ([]records.ConferenceRecordSummary, error)
	Read(ctx context.Context, uuid string) (records.RecordCard, error)
}

// WorkerSignaler описывает отправку browser SDP offer во внутренний worker.
type WorkerSignaler interface {
	Offer(ctx context.Context, recordID string, request records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error)
}

// Handler обрабатывает HTTP endpoints записей.
type Handler struct {
	service        Service
	workerSignaler WorkerSignaler
}

// NewHandler создает HTTP handler.
// Параметры:
// - service: use-case управления записью.
// - workerInternalURL: внутренний URL recorder-worker.
// Возвращает: Handler.
func NewHandler(service Service, workerInternalURL string) *Handler {
	return NewHandlerWithSignaler(service, workerinfra.NewClient(workerInternalURL))
}

// NewHandlerWithSignaler создает HTTP handler с явно переданным signaling-клиентом.
// Параметры:
// - service: use-case управления записью.
// - workerSignaler: клиент SDP signaling recorder-worker-а.
// Возвращает: Handler.
func NewHandlerWithSignaler(service Service, workerSignaler WorkerSignaler) *Handler {
	return &Handler{
		service:        service,
		workerSignaler: workerSignaler,
	}
}

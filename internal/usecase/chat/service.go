package chat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
)

type Repository interface {
	List(context.Context, string, string, string, int) (domain.Page, error)
	Send(context.Context, string, string, domain.SendRequest, string) (domain.Message, bool, error)
	Edit(context.Context, string, string, string, string) (domain.Message, error)
	Delete(context.Context, string, string, string) (domain.Message, error)
	ReadState(context.Context, string, string) (domain.ReadState, error)
	MarkRead(context.Context, string, string, string) (domain.ReadState, error)
	InitAttachment(context.Context, string, string, domain.InitRequest) (domain.Attachment, bool, error)
	ClaimUpload(context.Context, string, string, string, string) (domain.Attachment, error)
	CompleteUpload(context.Context, string, string, string, string, string) (domain.Attachment, error)
	AbortUpload(context.Context, string, string) error
	AttachmentForFinalize(context.Context, string, string, string) (domain.Attachment, error)
	FinalizeAttachment(context.Context, string, string, string, string) (domain.Attachment, error)
	DownloadAttachment(context.Context, string, string, string) (domain.Attachment, error)
	CleanupCandidates(context.Context, int) ([]domain.Attachment, error)
	CompleteCleanup(context.Context, string, string) error
}
type Storage interface {
	CheckAttachmentPrivacy(context.Context) error
	PutAttachment(context.Context, string, io.Reader, int64, string, string) error
	StatAttachment(context.Context, string) (int64, string, string, error)
	AttachmentDownloadURL(context.Context, string, string, time.Duration) (string, error)
	CleanAttachmentObjects(context.Context, string, string) error
}
type Events interface {
	Broadcast(context.Context, realtime.Envelope) error
	SendToParticipant(context.Context, string, string, realtime.Envelope) error
}
type Service struct {
	repo    Repository
	storage Storage
	events  Events
	uploads chan struct{}
}

func NewService(ctx context.Context, repo Repository, storage Storage, events Events) (*Service, error) {
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := storage.CheckAttachmentPrivacy(check); err != nil {
		return nil, err
	}
	return &Service{repo: repo, storage: storage, events: events, uploads: make(chan struct{}, 4)}, nil
}
func (s *Service) List(ctx context.Context, user, conference, cursor string, limit int) (domain.Page, error) {
	return s.repo.List(ctx, user, conference, cursor, limit)
}
func (s *Service) Send(ctx context.Context, user, conference string, request domain.SendRequest) (domain.Message, bool, error) {
	request, fingerprint, err := domain.NormalizeSend(request)
	if err != nil {
		return domain.Message{}, false, err
	}
	message, created, err := s.repo.Send(ctx, user, conference, request, fingerprint)
	if err == nil {
		s.publish(ctx, "chat.message.created", message)
	}
	return message, created, err
}
func (s *Service) Edit(ctx context.Context, user, conference, id string, request domain.EditRequest) (domain.Message, error) {
	text, err := domain.NormalizeText(request.Text, true)
	if err != nil {
		return domain.Message{}, err
	}
	message, err := s.repo.Edit(ctx, user, conference, id, text)
	if err == nil {
		s.publish(ctx, "chat.message.updated", message)
	}
	return message, err
}
func (s *Service) Delete(ctx context.Context, user, conference, id string) (domain.Message, error) {
	message, err := s.repo.Delete(ctx, user, conference, id)
	if err == nil {
		s.publish(ctx, "chat.message.deleted", message)
	}
	return message, err
}
func (s *Service) publish(ctx context.Context, kind string, message domain.Message) {
	if s.events != nil {
		if err := s.events.Broadcast(ctx, realtime.Event(kind, message.ConferenceID, message)); err != nil {
			slog.Warn("chat realtime delivery deferred to client refresh", "conference_id", message.ConferenceID, "event_type", kind)
		}
	}
}
func (s *Service) ReadState(ctx context.Context, user, conference string) (domain.ReadState, error) {
	return s.repo.ReadState(ctx, user, conference)
}
func (s *Service) MarkRead(ctx context.Context, user, conference string, request domain.ReadRequest) (domain.ReadState, error) {
	id, err := domain.UUID(request.MessageID)
	if err != nil {
		return domain.ReadState{}, err
	}
	state, err := s.repo.MarkRead(ctx, user, conference, id)
	if err == nil && s.events != nil {
		_ = s.events.SendToParticipant(ctx, conference, state.ParticipantID, realtime.Event("chat.read.updated", conference, state))
	}
	return state, err
}
func (s *Service) InitAttachment(ctx context.Context, user, conference string, request domain.InitRequest) (domain.Attachment, bool, error) {
	request, err := domain.NormalizeInit(request)
	if err != nil {
		return domain.Attachment{}, false, err
	}
	return s.repo.InitAttachment(ctx, user, conference, request)
}

func ValidateContent(data []byte, attachment domain.Attachment) error {
	if int64(len(data)) != attachment.Size || len(data) == 0 || int64(len(data)) > domain.MaxAttachmentBytes {
		return apperrors.New(apperrors.ErrInvalidInput, "uploaded size differs from the declared attachment size")
	}
	detected := strings.Split(http.DetectContentType(data), ";")[0]
	expected := attachment.MimeType
	if expected == "text/csv" {
		expected = "text/plain"
	}
	if detected != expected {
		return apperrors.New(apperrors.ErrInvalidInput, "file content does not match its allowed type")
	}
	if expected == "text/plain" && (!utf8.Valid(data) || bytes.ContainsRune(data, 0)) {
		return apperrors.New(apperrors.ErrInvalidInput, "text attachments must be valid UTF-8")
	}
	if expected == "image/jpeg" || expected == "image/png" {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
			return apperrors.New(apperrors.ErrInvalidInput, "invalid image or image dimensions exceed the limit")
		}
	}
	return nil
}
func (s *Service) Upload(ctx context.Context, user, conference, id string, reader io.Reader) (result domain.Attachment, err error) {
	select {
	case s.uploads <- struct{}{}:
		defer func() { <-s.uploads }()
	default:
		return result, apperrors.New(apperrors.ErrUnavailable, "attachment upload capacity is busy")
	}
	op, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	token := uuid.NewString()
	a, err := s.repo.ClaimUpload(op, user, conference, id, token)
	if err != nil {
		return result, err
	}
	defer func() {
		if err != nil && a.UploadedAt == nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = s.repo.AbortUpload(cleanup, id, token)
		}
	}()
	data, err := io.ReadAll(io.LimitReader(reader, domain.MaxAttachmentBytes+1))
	if err != nil {
		return result, apperrors.New(apperrors.ErrInvalidInput, "attachment upload was interrupted")
	}
	if err = ValidateContent(data, a); err != nil {
		return result, err
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	if a.UploadedAt != nil {
		if a.Checksum != checksum {
			return result, apperrors.New(apperrors.ErrConflict, "attachment already contains different bytes")
		}
		return a, nil
	}
	if err = s.storage.PutAttachment(op, a.ObjectKey, bytes.NewReader(data), int64(len(data)), a.MimeType, checksum); err != nil {
		return result, apperrors.ErrUnavailable
	}
	return s.repo.CompleteUpload(op, user, conference, id, token, checksum)
}
func (s *Service) FinalizeAttachment(ctx context.Context, user, conference, id string) (domain.Attachment, error) {
	a, err := s.repo.AttachmentForFinalize(ctx, user, conference, id)
	if err != nil {
		return a, err
	}
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	size, mime, checksum, err := s.storage.StatAttachment(op, a.ObjectKey)
	if err != nil {
		return domain.Attachment{}, apperrors.ErrUnavailable
	}
	if size != a.Size || mime != a.MimeType || checksum != a.Checksum {
		return domain.Attachment{}, apperrors.New(apperrors.ErrConflict, "stored attachment validation failed")
	}
	return s.repo.FinalizeAttachment(ctx, user, conference, id, a.ObjectKey)
}
func (s *Service) Download(ctx context.Context, user, conference, id string) (string, time.Time, error) {
	a, err := s.repo.DownloadAttachment(ctx, user, conference, id)
	if err != nil {
		return "", time.Time{}, err
	}
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	url, err := s.storage.AttachmentDownloadURL(op, a.ObjectKey, a.Filename, domain.DownloadTTL)
	if err != nil {
		return "", time.Time{}, apperrors.ErrUnavailable
	}
	return url, time.Now().UTC().Add(domain.DownloadTTL), nil
}
func (s *Service) Cleanup(ctx context.Context) error {
	rows, err := s.repo.CleanupCandidates(ctx, 100)
	if err != nil {
		return err
	}
	for _, a := range rows {
		keep := ""
		if a.Status == "attached" {
			keep = a.ObjectKey
		}
		if err := s.storage.CleanAttachmentObjects(ctx, a.Prefix(), keep); err != nil {
			return err
		}
		if err := s.repo.CompleteCleanup(ctx, a.ID, a.ObjectKey); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		op, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := s.Cleanup(op)
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Warn("attachment orphan cleanup will retry")
		}
	}
}

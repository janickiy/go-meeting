package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	mediadomain "github.com/janickiy/go-recorder/internal/domain/media"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/operations"
)

// handleMessage owns protocol dispatch. false ends the reader; recoverable
// request failures use the existing error envelope and keep the socket alive.
// Heartbeat deadlines, rate limiting and connection teardown stay in read.
func (c *client) handleMessage(raw []byte) bool {
	event, code := decodeEnvelope(raw, c.session.ConferenceID)
	if code != "" {
		c.failure(event.ID, code)
		return true
	}
	if c.authorize() != nil {
		return false
	}
	operations.WSMessage(event.Type)
	if strings.HasPrefix(event.Type, "media.") {
		c.handleMedia(event)
		return true
	}
	signal, code := decodeSignal(event, c.session, c.handler.cfg)
	if code != "" {
		c.failure(event.ID, code)
		return true
	}
	forward := domain.Event(event.Type, c.session.ConferenceID, signal)
	forward.ReplyTo = event.ID
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err := c.handler.hub.SendToConnection(ctx, c.session, signal.TargetConnectionID, forward)
	cancel()
	if err != nil {
		if errors.Is(err, apperrors.ErrForbidden) || errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrConflict) {
			c.failure(event.ID, "target_unavailable")
			return true
		}
		c.Stop("broker_unavailable")
		return false
	}
	ack := domain.Event("ack", c.session.ConferenceID, map[string]string{"type": event.Type})
	ack.ReplyTo = event.ID
	c.Offer(ack)
	return true
}

func (c *client) handleMedia(event domain.Envelope) {
	if c.handler.media == nil {
		c.failure(event.ID, "media_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(operations.WithID(context.Background(), event.ID), 10*time.Second)
	err := c.handler.media.Handle(ctx, c.session, c.expiresAt, event)
	cancel()
	if err != nil {
		c.failure(event.ID, mediadomain.ErrorCode(err))
	}
}

func decodeEnvelope(raw []byte, conferenceID string) (domain.Envelope, string) {
	var event domain.Envelope
	if !utf8.Valid(raw) || strictJSON(raw, &event) != nil {
		// Discard partially decoded IDs, as the reader did before extraction.
		return domain.Envelope{}, "invalid_message"
	}
	if parsed, err := uuid.Parse(event.ID); err != nil || parsed == uuid.Nil || event.Version != 1 || event.ConferenceID != conferenceID || event.Timestamp.IsZero() || event.ReplyTo != "" {
		return event, "invalid_envelope"
	}
	return event, ""
}

// decodeSignal validates legacy signaling, normalizes the target, and injects
// only the authenticated physical session identity. No transport side effects.
func decodeSignal(event domain.Envelope, session domain.Session, cfg config.RealtimeConfig) (domain.Signal, string) {
	if event.Type != "webrtc.offer" && event.Type != "webrtc.answer" && event.Type != "webrtc.ice" {
		return domain.Signal{}, "unsupported_event"
	}
	var signal domain.Signal
	if strictJSON(event.Data, &signal) != nil || signal.SenderConnectionID != "" || signal.SenderParticipantID != "" {
		return domain.Signal{}, "invalid_signal"
	}
	target, err := uuid.Parse(signal.TargetConnectionID)
	if err != nil || target == uuid.Nil || target.String() == session.ConnectionID {
		return domain.Signal{}, "invalid_target"
	}
	signal.TargetConnectionID = target.String()
	if event.Type == "webrtc.ice" {
		var candidate map[string]json.RawMessage
		if signal.SDP != "" || len(signal.Candidate) == 0 || len(signal.Candidate) > cfg.ICEBytes || json.Unmarshal(signal.Candidate, &candidate) != nil || candidate == nil {
			return domain.Signal{}, "invalid_ice"
		}
	} else if len(signal.Candidate) != 0 || len(signal.SDP) == 0 || len(signal.SDP) > cfg.SDPBytes {
		return domain.Signal{}, "invalid_sdp"
	}
	signal.SenderConnectionID = session.ConnectionID
	signal.SenderParticipantID = session.ParticipantID
	return signal, ""
}

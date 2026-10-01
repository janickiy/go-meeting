package mediaworker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/media"
)

func (h *Handler) serviceCommand(w http.ResponseWriter, r *http.Request, operation string, cmd media.Command) {
	if !validUUID(cmd.ConferenceID) || !validUUID(cmd.Route.LeaseID) || cmd.Route.WorkerID != h.cfg.WorkerID || cmd.Route.Endpoint != h.cfg.WorkerInternalURL || cmd.MediaPeerID != "" || cmd.SDP != "" || len(cmd.Candidate) > 0 || cmd.Ticket != "" {
		h.fail(w, media.ErrInvalid)
		return
	}
	if !h.ready.Load() {
		h.fail(w, media.ErrUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.OperationTimeout)
	defer cancel()
	owner, err := h.registry.GetOwner(ctx, cmd.ConferenceID)
	if err != nil || !sameRoute(owner, cmd.Route) {
		h.fail(w, media.ErrOwnership)
		return
	}
	unlock, err := h.lockRoom(ctx, cmd.ConferenceID)
	if err != nil {
		h.fail(w, err)
		return
	}
	defer unlock()
	switch operation {
	case "policy":
		if !validUUID(cmd.ParticipantID) || cmd.Policy == nil || cmd.Policy.Version < 0 {
			h.fail(w, media.ErrInvalid)
			return
		}
		engine, ok := h.engine.(interface {
			SetPolicy(context.Context, string, string, media.ParticipantPolicy) error
		})
		if !ok {
			h.fail(w, media.ErrUnavailable)
			return
		}
		err = engine.SetPolicy(ctx, cmd.ConferenceID, cmd.ParticipantID, *cmd.Policy)
	case "close":
		err = h.engine.CloseConference(ctx, cmd.ConferenceID)
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.json(w, http.StatusOK, media.Result{})
}

// Egress is a dedicated bounded stream between media-worker and recorder. The
// HTTP API process never receives media bytes, and writes have per-frame deadlines.
func (h *Handler) egress(w http.ResponseWriter, r *http.Request) {
	var req media.EgressRequest
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	raw, err := io.ReadAll(r.Body)
	if err != nil || strictJSON(raw, &req) != nil || !validUUID(req.RequestID) || r.Header.Get("X-Request-ID") != req.RequestID || !validUUID(req.ConferenceID) || !validUUID(req.RecordingID) || !validUUID(req.Route.LeaseID) || req.Route.WorkerID != h.cfg.WorkerID || req.Route.Endpoint != h.cfg.WorkerInternalURL || req.SegmentDurationSec < 1 || req.SegmentDurationSec > 30 {
		h.fail(w, media.ErrInvalid)
		return
	}
	engine, ok := h.engine.(interface {
		SubscribeRecording(context.Context, string, string) (media.EgressSubscription, error)
	})
	if !ok || !h.ready.Load() {
		h.fail(w, media.ErrUnavailable)
		return
	}
	setup, cancel := context.WithTimeout(r.Context(), h.cfg.OperationTimeout)
	defer cancel()
	unlock, err := h.lockRoom(setup, req.ConferenceID)
	if err != nil {
		h.fail(w, err)
		return
	}
	owner, err := h.registry.GetOwner(setup, req.ConferenceID)
	if err != nil || !sameRoute(owner, req.Route) {
		unlock()
		h.fail(w, media.ErrOwnership)
		return
	}
	renewedAt := time.Now()
	if h.registry.Renew(setup, req.ConferenceID, req.Route, h.cfg.OwnershipTTL) != nil {
		unlock()
		h.fail(w, media.ErrOwnership)
		return
	}
	h.mu.Lock()
	old, exists := h.leases[req.ConferenceID]
	h.mu.Unlock()
	if exists && !sameRoute(old.route, req.Route) {
		_ = h.engine.CloseConference(setup, req.ConferenceID)
	}
	h.mu.Lock()
	admitted := !h.fencing && !h.closing && h.workerDeadline.After(time.Now()) && renewedAt.Add(h.cfg.OwnershipTTL).After(time.Now())
	if admitted {
		h.leases[req.ConferenceID] = roomLease{route: req.Route, deadline: renewedAt.Add(h.cfg.OwnershipTTL)}
	}
	h.mu.Unlock()
	if !admitted {
		unlock()
		h.fail(w, media.ErrUnavailable)
		return
	}
	sub, err := engine.SubscribeRecording(setup, req.ConferenceID, req.RecordingID)
	unlock()
	if err != nil {
		h.fail(w, err)
		return
	}
	defer sub.Close()
	if !h.leaseValid(req.ConferenceID, req.Route) {
		h.fail(w, media.ErrOwnership)
		return
	}
	cancel()
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Request-ID", req.RequestID)
	control := http.NewResponseController(w)
	encoder := json.NewEncoder(w)
	write := func(frame media.EgressFrame) bool {
		if err := control.SetWriteDeadline(time.Now().Add(h.cfg.OperationTimeout)); err != nil {
			return false
		}
		if encoder.Encode(frame) != nil {
			return false
		}
		return control.Flush() == nil
	}
	heartbeat := time.NewTicker(time.Second)
	defer heartbeat.Stop()
	keyframes := time.NewTicker(time.Duration(req.SegmentDurationSec) * time.Second)
	defer keyframes.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-sub.Done():
			if sub.Err() != nil {
				_ = write(media.EgressFrame{Type: "error", Code: "recording_egress_failed", CapturedAt: time.Now().UnixNano()})
			}
			return
		case frame := <-sub.Frames():
			if !h.leaseValid(req.ConferenceID, req.Route) {
				return
			}
			if !write(frame) {
				return
			}
		case <-heartbeat.C:
			if !h.leaseValid(req.ConferenceID, req.Route) {
				return
			}
			sub.Ping()
		case <-keyframes.C:
			sub.Keyframes()
		}
	}
}

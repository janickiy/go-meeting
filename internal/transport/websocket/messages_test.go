package websocket

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
)

func TestEnvelopeValidationAndReplyCorrelation(t *testing.T) {
	room := uuid.NewString()
	for _, tc := range []struct {
		name   string
		mutate func(*domain.Envelope)
		code   string
	}{
		{"valid", func(*domain.Envelope) {}, ""},
		{"zero id", func(e *domain.Envelope) { e.ID = uuid.Nil.String() }, "invalid_envelope"},
		{"missing timestamp", func(e *domain.Envelope) { e.Timestamp = time.Time{} }, "invalid_envelope"},
		{"another room", func(e *domain.Envelope) { e.ConferenceID = uuid.NewString() }, "invalid_envelope"},
		{"version", func(e *domain.Envelope) { e.Version = 2 }, "invalid_envelope"},
		{"client reply", func(e *domain.Envelope) { e.ReplyTo = uuid.NewString() }, "invalid_envelope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := domain.Event("webrtc.offer", room, nil)
			tc.mutate(&e)
			raw, _ := json.Marshal(e)
			got, code := decodeEnvelope(raw, room)
			if code != tc.code || got.ID != e.ID {
				t.Fatalf("code=%s id=%s expected=%s/%s", code, got.ID, tc.code, e.ID)
			}
		})
	}
	for _, raw := range [][]byte{[]byte(`{"id":"partially-decoded","unknown":true}`), []byte(`{} {}`), {0xff}, []byte(`null`)} {
		e, code := decodeEnvelope(raw, room)
		if string(raw) == "null" {
			if code != "invalid_envelope" {
				t.Fatal(code)
			}
			continue
		}
		if code != "invalid_message" || e.ID != "" {
			t.Fatalf("partial malformed input echoed: %+v %s", e, code)
		}
	}
}

func TestLegacySignalValidationAndTrustedSender(t *testing.T) {
	session := domain.Session{ConnectionID: uuid.NewString(), ParticipantID: uuid.NewString()}
	target := uuid.NewString()
	cfg := config.RealtimeConfig{SDPBytes: 16, ICEBytes: 32}
	for _, tc := range []struct {
		name, kind string
		signal     domain.Signal
		code       string
	}{
		{"offer", "webrtc.offer", domain.Signal{TargetConnectionID: strings.ToUpper(target), SDP: "sdp"}, ""},
		{"answer", "webrtc.answer", domain.Signal{TargetConnectionID: target, SDP: "sdp"}, ""},
		{"ice", "webrtc.ice", domain.Signal{TargetConnectionID: target, Candidate: json.RawMessage(`{}`)}, ""},
		{"sender spoof", "webrtc.offer", domain.Signal{TargetConnectionID: target, SDP: "sdp", SenderConnectionID: uuid.NewString()}, "invalid_signal"},
		{"participant spoof", "webrtc.offer", domain.Signal{TargetConnectionID: target, SDP: "sdp", SenderParticipantID: uuid.NewString()}, "invalid_signal"},
		{"self target", "webrtc.offer", domain.Signal{TargetConnectionID: session.ConnectionID, SDP: "sdp"}, "invalid_target"},
		{"zero target", "webrtc.offer", domain.Signal{TargetConnectionID: uuid.Nil.String(), SDP: "sdp"}, "invalid_target"},
		{"empty sdp", "webrtc.offer", domain.Signal{TargetConnectionID: target}, "invalid_sdp"},
		{"large sdp", "webrtc.offer", domain.Signal{TargetConnectionID: target, SDP: strings.Repeat("s", 17)}, "invalid_sdp"},
		{"mixed payload", "webrtc.offer", domain.Signal{TargetConnectionID: target, SDP: "sdp", Candidate: json.RawMessage(`{}`)}, "invalid_sdp"},
		{"ice null", "webrtc.ice", domain.Signal{TargetConnectionID: target, Candidate: json.RawMessage(`null`)}, "invalid_ice"},
		{"ice array", "webrtc.ice", domain.Signal{TargetConnectionID: target, Candidate: json.RawMessage(`[]`)}, "invalid_ice"},
		{"ice sdp", "webrtc.ice", domain.Signal{TargetConnectionID: target, SDP: "sdp", Candidate: json.RawMessage(`{}`)}, "invalid_ice"},
		{"large ice", "webrtc.ice", domain.Signal{TargetConnectionID: target, Candidate: json.RawMessage(`{"candidate":"` + strings.Repeat("c", 32) + `"}`)}, "invalid_ice"},
		{"unknown event", "other", domain.Signal{}, "unsupported_event"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.signal)
			if err != nil {
				t.Fatal(err)
			}
			got, code := decodeSignal(domain.Envelope{Type: tc.kind, Data: data}, session, cfg)
			if code != tc.code {
				t.Fatalf("code=%s want=%s", code, tc.code)
			}
			if code == "" && (got.TargetConnectionID != target || got.SenderConnectionID != session.ConnectionID || got.SenderParticipantID != session.ParticipantID) {
				t.Fatalf("untrusted identity: %+v", got)
			}
		})
	}
	_, code := decodeSignal(domain.Envelope{Type: "webrtc.offer", Data: json.RawMessage(`{"unknown":true}`)}, session, cfg)
	if code != "invalid_signal" {
		t.Fatal(code)
	}
}

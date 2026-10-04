package app

import (
	"encoding/json"
	"testing"

	"github.com/janickiy/go-recorder/internal/domain/records"
)

// TestRecordingWorkerEventPreservesInitiator проверяет доставку инициатора без изменения
// типов событий старта, остановки и готовности и без раскрытия внутренних стадий обработки.
// @args t — исполнитель теста с сообщениями об ошибках.
func TestRecordingWorkerEventPreservesInitiator(t *testing.T) {
	owner, failure := "owner-id", "storage temporarily unavailable"
	for _, item := range []struct{ kind, status, public string }{
		{"recording.started", records.StatusRecording, records.StatusRecording},
		{"recording.stopping", records.StatusStopping, records.StatusStopping},
		{"recording.processing", records.StatusFinalizing, "processing"},
		{"recording.processing", records.StatusUploading, "processing"},
		{"recording.ready", records.StatusReady, records.StatusReady},
		{"recording.failed", records.StatusFailed, records.StatusFailed},
	} {
		t.Run(item.status, func(t *testing.T) {
			record := records.Record{UUID: "record-id", ConferenceID: "conference-id", RequestedBy: &owner, Mode: records.ModeAudioOnly, Status: item.status, ErrorMessage: &failure}
			event := recordingWorkerEvent(item.kind, record)
			var payload map[string]any
			if err := json.Unmarshal(event.Data, &payload); err != nil {
				t.Fatal(err)
			}
			if event.Type != item.kind || event.ConferenceID != record.ConferenceID || payload["requestedBy"] != owner || payload["recordingId"] != record.UUID || payload["conferenceId"] != record.ConferenceID || payload["mode"] != record.Mode || payload["status"] != item.public || payload["error"] != failure {
				t.Fatalf("unexpected worker event: %+v / %+v", event, payload)
			}
		})
	}
}

// TestRecordingWorkerEventDoesNotInventInitiator сохраняет отсутствие инициатора старых записей.
// @args t — исполнитель теста с сообщениями об ошибках.
func TestRecordingWorkerEventDoesNotInventInitiator(t *testing.T) {
	event := recordingWorkerEvent("recording.ready", records.Record{Status: records.StatusReady})
	var payload map[string]any
	if err := json.Unmarshal(event.Data, &payload); err != nil {
		t.Fatal(err)
	}
	if value, exists := payload["requestedBy"]; !exists || value != nil {
		t.Fatalf("unexpected initiator: %+v", payload)
	}
}

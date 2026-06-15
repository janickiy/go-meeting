package worker_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.svc-dev.net/board/go-recorder/internal/domain/records"
	workerinfra "git.svc-dev.net/board/go-recorder/internal/infrastructure/worker"
)

func TestClientSendsStartRecordCommand(t *testing.T) {
	var got records.Command
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/records/record-1/start" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer server.Close()

	client := workerinfra.NewClient(server.URL)
	if err := client.StartRecord(context.Background(), "record-1", 7); err != nil {
		t.Fatalf("StartRecord() error = %v", err)
	}

	if got.Type != "record.start" || got.RecordID != "record-1" || got.SegmentDurationSec != 7 {
		t.Fatalf("command = %+v", got)
	}
}

func TestClientSendsStopRecordCommand(t *testing.T) {
	var got records.Command
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/records/record-1/stop" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer server.Close()

	client := workerinfra.NewClient(server.URL)
	if err := client.StopRecord(context.Background(), "record-1", "client_stop"); err != nil {
		t.Fatalf("StopRecord() error = %v", err)
	}

	if got.Type != "record.stop" || got.RecordID != "record-1" || got.Reason != "client_stop" {
		t.Fatalf("command = %+v", got)
	}
}

func TestClientReturnsWorkerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "worker session is not prepared", http.StatusBadRequest)
	}))
	defer server.Close()

	client := workerinfra.NewClient(server.URL)
	if err := client.StartRecord(context.Background(), "record-1", 5); err == nil {
		t.Fatal("StartRecord() error is nil")
	}
}

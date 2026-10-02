package worker_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/janickiy/go-recorder/internal/domain/records"
	workerinfra "github.com/janickiy/go-recorder/internal/infrastructure/worker"
)

// TestClientSendsStartRecordCommand проверяет сценарий «клиент Sends запуск запись Command», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestClientSendsStartRecordCommand(t *testing.T) {
	var got records.Command
	server := httptest.NewServer(http.HandlerFunc( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - w (http.ResponseWriter): получатель HTTP-ответа.
		  - r (*http.Request): входящий HTTP-запрос.
		*/func(w http.ResponseWriter, r *http.Request) {
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

// TestClientSendsStopRecordCommand проверяет сценарий «клиент Sends остановка запись Command», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestClientSendsStopRecordCommand(t *testing.T) {
	var got records.Command
	server := httptest.NewServer(http.HandlerFunc( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - w (http.ResponseWriter): получатель HTTP-ответа.
		  - r (*http.Request): входящий HTTP-запрос.
		*/func(w http.ResponseWriter, r *http.Request) {
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

// TestClientReturnsWorkerError проверяет сценарий «клиент Returns воркер ошибка», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestClientReturnsWorkerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - w (http.ResponseWriter): получатель HTTP-ответа.
		  - _ (*http.Request): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
		*/func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "worker session is not prepared", http.StatusBadRequest)
		}))
	defer server.Close()

	client := workerinfra.NewClient(server.URL)
	if err := client.StartRecord(context.Background(), "record-1", 5); err == nil {
		t.Fatal("StartRecord() error is nil")
	}
}

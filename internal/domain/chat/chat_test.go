package chat

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestMessageValidationAndStableRetryFingerprint проверяет сценарий «сообщение проверка входных данных и Stable повторная попытка отпечаток запроса», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestMessageValidationAndStableRetryFingerprint(t *testing.T) {
	request := SendRequest{ClientRequestID: uuid.NewString(), Text: "  Привет 👋  ", AttachmentIDs: []string{uuid.NewString(), uuid.NewString()}}
	first, fingerprint, err := NormalizeSend(request)
	if err != nil || first.Text != "Привет 👋" {
		t.Fatal(first, err)
	}
	request.AttachmentIDs[0], request.AttachmentIDs[1] = request.AttachmentIDs[1], request.AttachmentIDs[0]
	_, retry, err := NormalizeSend(request)
	if err != nil || retry != fingerprint {
		t.Fatal("attachment order changed request identity")
	}
	for _, request := range []SendRequest{{ClientRequestID: uuid.NewString()}, {ClientRequestID: "bad", Text: "text"}, {ClientRequestID: uuid.NewString(), Text: strings.Repeat("я", 4001)}, {ClientRequestID: uuid.NewString(), Text: "x", ReplyTo: "bad"}, {ClientRequestID: uuid.NewString(), Text: "x", AttachmentIDs: []string{uuid.Nil.String()}}} {
		if _, _, err := NormalizeSend(request); err == nil {
			t.Fatal("accepted invalid request", request)
		}
	}
	if _, _, err := NormalizeSend(SendRequest{ClientRequestID: uuid.NewString(), Text: strings.Repeat("я", 4000)}); err != nil {
		t.Fatal(err)
	}
}

// TestAttachmentMetadataValidation проверяет сценарий «вложение Metadata проверка входных данных», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestAttachmentMetadataValidation(t *testing.T) {
	for _, name := range []string{"../secret.txt", "dir/file.txt", "dir\\file.txt", "evil.svg", "program.exe", "file\n.txt", "..hidden.txt", "", "image.jpg.exe", "file\u202e.txt"} {
		if _, err := NormalizeInit(InitRequest{ClientRequestID: uuid.NewString(), Filename: name, Size: 10}); err == nil {
			t.Fatal("unsafe name accepted", name)
		}
	}
	for _, size := range []int64{0, -1, MaxAttachmentBytes + 1} {
		if _, err := NormalizeInit(InitRequest{ClientRequestID: uuid.NewString(), Filename: "report.pdf", Size: size}); err == nil {
			t.Fatal("unsafe size accepted")
		}
	}
	if _, err := NormalizeInit(InitRequest{ClientRequestID: uuid.NewString(), Filename: "report.pdf", MimeType: "image/png", Size: 10}); err == nil {
		t.Fatal("MIME mismatch accepted")
	}
	a, err := NormalizeInit(InitRequest{ClientRequestID: uuid.NewString(), Filename: "  отчёт.csv  ", Size: 20})
	if err != nil || a.MimeType != "text/csv" || a.Filename != "отчёт.csv" {
		t.Fatal(a, err)
	}
}

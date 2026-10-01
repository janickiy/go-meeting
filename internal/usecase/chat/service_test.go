package chat

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	domain "github.com/janickiy/go-recorder/internal/domain/chat"
)

// TestValidateAttachmentBytesNotBrowserMIME проверяет сценарий «Validate вложение байты не браузер MIME», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestValidateAttachmentBytesNotBrowserMIME(t *testing.T) {
	for _, sample := range []struct {
		mime string
		data []byte
	}{
		{"image/png", []byte("<html>active content</html>")},
		{"application/pdf", []byte("not a PDF")},
		{"text/plain", []byte{0, 1, 2}},
		{"image/png", []byte{137, 80, 78, 71, 13, 10, 26, 10}},
	} {
		if err := ValidateContent(sample.data, domain.Attachment{MimeType: sample.mime, Size: int64(len(sample.data))}); err == nil {
			t.Fatal("unsafe content accepted", sample.mime)
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if err := ValidateContent(data.Bytes(), domain.Attachment{MimeType: "image/png", Size: int64(data.Len())}); err != nil {
		t.Fatal(err)
	}
	text := []byte("Привет, мир\n")
	if err := ValidateContent(text, domain.Attachment{MimeType: "text/plain", Size: int64(len(text))}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateContent(text, domain.Attachment{MimeType: "text/plain", Size: int64(len(text) + 1)}); err == nil {
		t.Fatal("declared size mismatch accepted")
	}
}

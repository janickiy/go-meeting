package integrations

import (
	"strings"
	"testing"
	"time"
)

func TestRenderInvitationEmailEscapesAndIncludesUTC(t *testing.T) {
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.FixedZone("offset", 3*3600))
	message, err := RenderInvitationEmail("stable-key", "guest@example.org", "<script>meeting</script>", "<img src=x>", &at, "https://meet.example/i/secret-code")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(message.HTML, "<script>") || strings.Contains(message.HTML, "<img src=x>") {
		t.Fatal("unescaped user content")
	}
	for _, fragment := range []string{"05.10.2026 09:00 UTC", "https://meet.example/i/secret-code", "регистрация не требуется"} {
		if !strings.Contains(message.Text, fragment) {
			t.Fatal("missing invitation detail", fragment)
		}
	}
	if message.IdempotencyKey != "stable-key" || message.To != "guest@example.org" {
		t.Fatal("delivery identifiers changed")
	}
}

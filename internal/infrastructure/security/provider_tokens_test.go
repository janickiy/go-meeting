package security

import (
	"encoding/base64"
	"strings"
	"testing"
)

// TestProviderTokenBinding проверяет непереносимость ciphertext между владельцами и отсутствие plaintext.
// @parameters: t — контекст изолированной проверки.
func TestProviderTokenBinding(t *testing.T) {
	cipher, err := NewProviderTokens(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32))))
	if err != nil {
		t.Fatal(err)
	}
	a, err := cipher.Encrypt("secret-refresh-token", "alice:calendar:id:refresh")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := cipher.Encrypt("secret-refresh-token", "alice:calendar:id:refresh")
	if a == b || strings.Contains(a, "secret") {
		t.Fatal("nonce/plaintext failure")
	}
	value, err := cipher.Decrypt(a, "alice:calendar:id:refresh")
	if err != nil || value != "secret-refresh-token" {
		t.Fatal("decrypt failure", err)
	}
	for _, binding := range []string{"bob:calendar:id:refresh", "alice:calendar:other:refresh", "alice:calendar:id:access"} {
		if _, err := cipher.Decrypt(a, binding); err == nil {
			t.Fatal("binding bypass")
		}
	}
	if _, err := cipher.Decrypt(a[:len(a)-2]+"xx", "alice:calendar:id:refresh"); err == nil {
		t.Fatal("tamper accepted")
	}
	if _, err := NewProviderTokens("not-base64"); err == nil {
		t.Fatal("invalid key accepted")
	}
	var disabled *ProviderTokens
	if _, err := disabled.Encrypt("secret", "binding"); err == nil {
		t.Fatal("disabled encryption accepted")
	}
}

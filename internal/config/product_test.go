package config

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

// TestStageSevenPrivacyDefaults проверяет, что запуск приложения не отправляет пользовательские данные наружу.
// @args t — контекст проверки конфигурации по умолчанию.
func TestStageSevenPrivacyDefaults(t *testing.T) {
	for _, name := range []string{"TRANSCRIPTION_ENABLED", "AI_SUMMARY_ENABLED", "STT_PROVIDER_MODE", "AI_PROVIDER_MODE", "EMAIL_PROVIDER_MODE", "PUSH_PROVIDER_MODE", "CALENDAR_PROVIDER_MODE", "REMINDER_OFFSETS"} {
		t.Setenv(name, "")
	}
	c, err := LoadStageSeven(true)
	if err != nil {
		t.Fatal(err)
	}
	if c.STTEnabled || c.AIEnabled || c.STT.Mode != "noop" || c.AI.Mode != "noop" || len(c.ReminderOffsets) != 2 {
		t.Fatal("unsafe feature defaults")
	}
}

func TestStageSevenSMTPConfiguration(t *testing.T) {
	t.Setenv("EMAIL_PROVIDER_MODE", "smtp")
	t.Setenv("PUBLIC_FRONTEND_URL", "https://meet.example.test")
	t.Setenv("SMTP_HOST", "smtp.example.test")
	t.Setenv("SMTP_USERNAME", "sender@example.test")
	t.Setenv("SMTP_PASSWORD", "private-test-password")
	t.Setenv("SMTP_FROM", "Meetrix <sender@example.test>")
	t.Setenv("SMTP_PORT", "")
	t.Setenv("SMTP_TLS_MODE", "")
	t.Setenv("SMTP_TIMEOUT", "")
	c, err := LoadStageSeven(false)
	if err != nil || c.SMTP.Port != 465 || c.SMTP.TLSMode != "tls" || c.SMTP.Timeout != 15*time.Second || c.Email.Mode != "smtp" {
		t.Fatal("secure SMTP defaults missing", err)
	}
	t.Setenv("SMTP_TLS_MODE", "starttls")
	c, err = LoadStageSeven(false)
	if err != nil || c.SMTP.Port != 587 {
		t.Fatal("STARTTLS default port missing", err)
	}
	for _, tc := range []struct{ key, value string }{
		{"SMTP_HOST", "smtp://smtp.example.test"},
		{"SMTP_HOST", "smtp.example.test:587"},
		{"SMTP_HOST", "user:private-test-password@smtp.example.test"},
		{"SMTP_PORT", "0"},
		{"SMTP_PORT", "65536"},
		{"SMTP_TLS_MODE", "none"},
		{"SMTP_USERNAME", ""},
		{"SMTP_PASSWORD", ""},
		{"SMTP_FROM", "sender@example.test\r\nBcc: stolen@example.test"},
		{"SMTP_FROM", "one@example.test, two@example.test"},
		{"SMTP_TIMEOUT", "2m"},
	} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			_, err := LoadStageSeven(false)
			if err == nil || strings.Contains(err.Error(), "private-test-password") || strings.Contains(err.Error(), "stolen@example.test") {
				t.Fatal("invalid SMTP configuration accepted or secret exposed")
			}
		})
	}
	t.Setenv("PUSH_PROVIDER_MODE", "smtp")
	if _, err := LoadStageSeven(false); err == nil {
		t.Fatal("SMTP accepted for a different channel")
	}
}

// TestStageSevenRejectsUnsafeConfiguration отклоняет подставных провайдеров в рабочей среде, неверные числа и неполный OAuth.
// @args t — контекст негативных сценариев.
func TestStageSevenRejectsUnsafeConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, key, value string
		local            bool
	}{
		{"mock production", "STT_PROVIDER_MODE", "mock", false},
		{"invalid flag", "TRANSCRIPTION_ENABLED", "maybe", true},
		{"disabled provider", "TRANSCRIPTION_ENABLED", "true", true},
		{"workers", "TRANSCRIPTION_CONCURRENCY", "200", true},
		{"segments", "TRANSCRIPTION_MAX_SEGMENTS", "10001", true},
		{"chunks", "AI_MAX_CHUNKS", "513", true},
		{"chunk size", "AI_CHUNK_RUNES", "50001", true},
		{"duplicate offsets", "REMINDER_OFFSETS", "15m,15m", true},
		{"oauth incomplete", "CALENDAR_OAUTH_CLIENT_ID", "client", true},
		{"oauth secret alone", "CALENDAR_OAUTH_CLIENT_SECRET", "client-secret", true},
		{"model size", "AI_PROVIDER_MODEL", strings.Repeat("m", 101), true},
		{"key", "PROVIDER_TOKEN_ENCRYPTION_KEY", "not base64", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := LoadStageSeven(tc.local); err == nil {
				t.Fatal("unsafe config accepted")
			}
		})
	}
}

// TestStageSevenRejectsReusedJWTKey запрещает повторное использование секрета JWT как ключа AES в base64.
// @args t — контекст проверки независимости ключей разных назначений.
func TestStageSevenRejectsReusedJWTKey(t *testing.T) {
	key := strings.Repeat("j", 32)
	t.Setenv("JWT_SECRET", key)
	t.Setenv("PROVIDER_TOKEN_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte(key)))
	if _, err := LoadStageSeven(true); err == nil {
		t.Fatal("JWT key reused for provider encryption")
	}
}

// TestStageSevenHTTPRequiresTrustedEndpointAndEncryption проверяет явную подготовку внешнего канала.
// @args t — контекст проверки секретов и адресов без их вывода.
func TestStageSevenHTTPRequiresTrustedEndpointAndEncryption(t *testing.T) {
	t.Setenv("PUSH_PROVIDER_MODE", "http")
	t.Setenv("PUSH_PROVIDER_ENDPOINT", "https://gateway.example")
	t.Setenv("PUSH_PROVIDER_TOKEN", strings.Repeat("a", 32))
	if _, err := LoadStageSeven(true); err == nil {
		t.Fatal("push accepted without encryption")
	}
	t.Setenv("PROVIDER_TOKEN_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("b", 32))))
	if _, err := LoadStageSeven(true); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PUSH_PROVIDER_ENDPOINT", "https://user:password@gateway.example")
	if _, err := LoadStageSeven(true); err == nil {
		t.Fatal("credential URL accepted")
	}
}

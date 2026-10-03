package config

import (
	"encoding/base64"
	"strings"
	"testing"
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

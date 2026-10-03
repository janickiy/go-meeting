package config

import "testing"

// TestStageEightDefaultsAndBoundaries проверяет явное включение и конечные бюджеты вспомогательных компонентов.
// @args t — исполнитель теста конфигурации.
func TestStageEightDefaultsAndBoundaries(t *testing.T) {
	for _, key := range []string{"LIVE_STT_ENABLED", "EMBEDDINGS_ENABLED", "MEETING_ANALYTICS_ENABLED", "LIVE_STT_PROVIDER_MODE", "EMBEDDING_PROVIDER_MODE"} {
		t.Setenv(key, "")
	}
	c, err := LoadStageEight(true)
	if err != nil || c.LiveEnabled || c.EmbeddingsEnabled || c.AnalyticsEnabled || c.LiveSessions != 10 {
		t.Fatal("unsafe defaults", err)
	}
	for _, tc := range []struct {
		key, value string
		local      bool
	}{{"LIVE_MAX_SESSIONS", "101", true}, {"LIVE_MAX_DURATION", "25h", true}, {"EMBEDDING_DIMENSIONS", "2001", true}, {"LIVE_STT_ENABLED", "true", true}, {"LIVE_STT_PROVIDER_MODE", "mock", false}, {"EMBEDDING_PROVIDER_MODE", "mock", false}} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := LoadStageEight(tc.local); err == nil {
				t.Fatal("unsafe config accepted")
			}
		})
	}
}

// TestStageEightProviderTransport запрещает небезопасный канал в рабочей среде и секреты в URL.
// @args t — исполнитель негативных сценариев.
func TestStageEightProviderTransport(t *testing.T) {
	t.Setenv("LIVE_STT_PROVIDER_MODE", "websocket")
	t.Setenv("LIVE_STT_PROVIDER_TOKEN", "test-only-credential-value")
	for _, endpoint := range []string{"ws://gateway.example", "wss://user:pass@gateway.example", "wss://gateway.example?token=value"} {
		t.Setenv("LIVE_STT_PROVIDER_ENDPOINT", endpoint)
		if _, err := LoadStageEight(false); err == nil {
			t.Fatal("unsafe endpoint accepted")
		}
	}
	t.Setenv("LIVE_STT_PROVIDER_ENDPOINT", "wss://gateway.example/live")
	if _, err := LoadStageEight(false); err != nil {
		t.Fatal(err)
	}
}

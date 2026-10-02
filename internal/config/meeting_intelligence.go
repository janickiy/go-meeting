package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// StageEightConfig ограничивает вспомогательную обработку речи, векторов и аналитики.
// Передача аудио и текста провайдерам требует явного включения; mock запрещён в production.
type StageEightConfig struct {
	LiveEnabled, EmbeddingsEnabled, AnalyticsEnabled                           bool
	Live, Embeddings                                                           ProviderConfig
	EmbeddingModel, EmbeddingVersion                                           string
	LivePort, LiveConferences, LiveSessions, LiveQueue, LiveAttempts           int
	EmbeddingDimensions, EmbeddingBatch, EmbeddingWorkers, EmbeddingChunkRunes int
	MaxCaptionRunes, MaxFinalCaptions                                          int
	LiveMaxDuration, ProviderTimeout, EmbeddingTimeout                         time.Duration
}

// LoadStageEight читает операторские настройки без вывода токенов в ошибки.
// @args local — разрешение локальных HTTP/WebSocket и тестовых адаптеров.
// @return конфигурация с конечными пределами либо ошибка.
func LoadStageEight(local bool) (StageEightConfig, error) {
	c := StageEightConfig{}
	var err error
	for key, p := range map[string]*bool{"LIVE_STT_ENABLED": &c.LiveEnabled, "EMBEDDINGS_ENABLED": &c.EmbeddingsEnabled, "MEETING_ANALYTICS_ENABLED": &c.AnalyticsEnabled} {
		*p, err = strconv.ParseBool(env(key, "false"))
		if err != nil {
			return c, fmt.Errorf("%s must be boolean", key)
		}
	}
	c.Live = ProviderConfig{Mode: env("LIVE_STT_PROVIDER_MODE", "noop"), Endpoint: os.Getenv("LIVE_STT_PROVIDER_ENDPOINT"), Token: os.Getenv("LIVE_STT_PROVIDER_TOKEN")}
	c.Embeddings = ProviderConfig{Mode: env("EMBEDDING_PROVIDER_MODE", "noop"), Endpoint: os.Getenv("EMBEDDING_PROVIDER_ENDPOINT"), Token: os.Getenv("EMBEDDING_PROVIDER_TOKEN")}
	for name, p := range map[string]ProviderConfig{"LIVE_STT": c.Live, "EMBEDDING": c.Embeddings} {
		expected := "http"
		if name == "LIVE_STT" {
			expected = "websocket"
		}
		if p.Mode != "noop" && p.Mode != "mock" && p.Mode != expected {
			return c, fmt.Errorf("invalid %s_PROVIDER_MODE", name)
		}
		if p.Mode == "mock" && !local {
			return c, fmt.Errorf("%s mock is forbidden in production", name)
		}
		if p.Mode == expected {
			u, e := url.Parse(p.Endpoint)
			secure, dev := "https", "http"
			if name == "LIVE_STT" {
				secure, dev = "wss", "ws"
			}
			if e != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || (u.Scheme != secure && !(local && u.Scheme == dev)) || len(p.Token) < 16 {
				return c, fmt.Errorf("invalid %s endpoint or token", name)
			}
		}
	}
	if c.LiveEnabled && c.Live.Mode == "noop" || c.EmbeddingsEnabled && c.Embeddings.Mode == "noop" {
		return c, fmt.Errorf("enabled intelligence requires a provider")
	}
	c.EmbeddingModel = env("EMBEDDING_MODEL", "configured-model")
	c.EmbeddingVersion = env("EMBEDDING_VERSION", "v1")
	for _, s := range []string{c.EmbeddingModel, c.EmbeddingVersion} {
		if len(s) < 1 || len(s) > 100 || strings.ContainsAny(s, "\x00\r\n") {
			return c, fmt.Errorf("invalid embedding model/version")
		}
	}
	for _, f := range []struct {
		k                  string
		p                  *int
		initial, low, high int
	}{
		{"LIVE_WORKER_PORT", &c.LivePort, 8093, 1, 65535}, {"LIVE_MAX_CONFERENCES", &c.LiveConferences, 2, 1, 32},
		{"LIVE_MAX_SESSIONS", &c.LiveSessions, 10, 1, 100}, {"LIVE_AUDIO_QUEUE", &c.LiveQueue, 100, 8, 500}, {"LIVE_MAX_ATTEMPTS", &c.LiveAttempts, 3, 1, 5},
		{"LIVE_MAX_CAPTION_RUNES", &c.MaxCaptionRunes, 2000, 100, 4000}, {"LIVE_MAX_FINAL_CAPTIONS", &c.MaxFinalCaptions, 10000, 100, 50000},
		{"EMBEDDING_DIMENSIONS", &c.EmbeddingDimensions, 64, 8, 2000}, {"EMBEDDING_BATCH_SIZE", &c.EmbeddingBatch, 16, 1, 64},
		{"EMBEDDING_CONCURRENCY", &c.EmbeddingWorkers, 1, 1, 8}, {"EMBEDDING_CHUNK_RUNES", &c.EmbeddingChunkRunes, 1200, 128, 4000},
	} {
		*f.p, err = strconv.Atoi(env(f.k, strconv.Itoa(f.initial)))
		if err != nil || *f.p < f.low || *f.p > f.high {
			return c, fmt.Errorf("%s must be %d..%d", f.k, f.low, f.high)
		}
	}
	for _, f := range []struct {
		k                  string
		p                  *time.Duration
		initial, low, high time.Duration
	}{
		{"LIVE_MAX_DURATION", &c.LiveMaxDuration, 2 * time.Hour, time.Minute, 24 * time.Hour},
		{"LIVE_PROVIDER_TIMEOUT", &c.ProviderTimeout, 5 * time.Second, time.Second, 30 * time.Second},
		{"EMBEDDING_TIMEOUT", &c.EmbeddingTimeout, time.Minute, time.Second, 10 * time.Minute},
	} {
		*f.p, err = time.ParseDuration(env(f.k, f.initial.String()))
		if err != nil || *f.p < f.low || *f.p > f.high {
			return c, fmt.Errorf("invalid %s", f.k)
		}
	}
	return c, nil
}

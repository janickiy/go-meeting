package config

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ProviderConfig описывает серверный адаптер; endpoint и token никогда не принимаются от пользователя.
type ProviderConfig struct {
	Mode, Endpoint, Token string
}

// StageSevenConfig задаёт бюджет отдельного продуктового работника и внешние интеграции.
// STT/AI выключены по умолчанию; MockAllowed запрещает фиктивные внешние результаты в production.
// Ключ шифрования независим от JWT и используется только для устройств и календарных токенов.
type StageSevenConfig struct {
	PublicURL, EncryptionKey, TempRoot                            string
	Email, Push, Calendar, STT, AI                                ProviderConfig
	AIModel                                                       string
	OAuthAuthURL, OAuthTokenURL, OAuthRevokeURL                   string
	OAuthClientID, OAuthClientSecret, OAuthRedirectURL            string
	OAuthScopes                                                   []string
	ReminderOffsets                                               []time.Duration
	STTEnabled, AIEnabled, MockAllowed                            bool
	Port, DeliveryWorkers, CalendarWorkers, STTWorkers, AIWorkers int
	MaxDurationSec, MaxSegments, ChunkRunes, MaxChunks            int
	AIConcurrency, MaxReprocess, MaxAttempts                      int
	MaxVideoBytes, MaxAudioBytes, MaxTranscriptBytes              int64
	PollInterval, ProviderTimeout, STTTimeout, AITimeout          time.Duration
	ReprocessCooldown                                             time.Duration
}

// LoadStageSeven читает только продуктовую конфигурацию и отклоняет неоднозначные env-значения.
// @args local — локальный/test режим, разрешающий HTTP тестового gateway и mock.
// @return проверенные пределы, адреса и секреты либо ошибка без значений секретов.
func LoadStageSeven(local bool) (StageSevenConfig, error) {
	c := StageSevenConfig{MockAllowed: local, TempRoot: env("PRODUCT_TEMP_PATH", os.TempDir()), AIModel: env("AI_PROVIDER_MODEL", "configured-model")}
	if !utf8.ValidString(c.AIModel) || strings.ContainsRune(c.AIModel, 0) || utf8.RuneCountInString(c.AIModel) > 100 {
		return c, fmt.Errorf("AI_PROVIDER_MODEL requires valid text within 100 characters")
	}
	for prefix, target := range map[string]*ProviderConfig{"EMAIL": &c.Email, "PUSH": &c.Push, "CALENDAR": &c.Calendar, "STT": &c.STT, "AI": &c.AI} {
		*target = ProviderConfig{Mode: env(prefix+"_PROVIDER_MODE", "noop"), Endpoint: os.Getenv(prefix + "_PROVIDER_ENDPOINT"), Token: os.Getenv(prefix + "_PROVIDER_TOKEN")}
		if target.Mode != "noop" && target.Mode != "mock" && target.Mode != "http" {
			return c, fmt.Errorf("%s_PROVIDER_MODE must be noop, mock or http", prefix)
		}
		if target.Mode == "mock" && !local {
			return c, fmt.Errorf("%s mock provider is forbidden in production", prefix)
		}
		if target.Mode == "http" {
			if err := stageSevenURL(prefix+"_PROVIDER_ENDPOINT", target.Endpoint, local); err != nil {
				return c, err
			}
			if len(target.Token) < 16 {
				return c, fmt.Errorf("%s_PROVIDER_TOKEN requires 16+ bytes", prefix)
			}
		}
	}
	var err error
	for key, target := range map[string]*bool{"TRANSCRIPTION_ENABLED": &c.STTEnabled, "AI_SUMMARY_ENABLED": &c.AIEnabled} {
		raw := env(key, "false")
		*target, err = strconv.ParseBool(raw)
		if err != nil {
			return c, fmt.Errorf("%s must be a boolean", key)
		}
	}
	if (c.STTEnabled && c.STT.Mode == "noop") || (c.AIEnabled && c.AI.Mode == "noop") {
		return c, fmt.Errorf("enabled transcription/AI require configured providers")
	}
	c.PublicURL = os.Getenv("PUBLIC_FRONTEND_URL")
	if c.PublicURL == "" && local {
		c.PublicURL = "http://localhost:5173"
	}
	if c.PublicURL != "" {
		if err := stageSevenURL("PUBLIC_FRONTEND_URL", c.PublicURL, local); err != nil {
			return c, err
		}
	} else if c.Email.Mode != "noop" || c.Calendar.Mode != "noop" || c.Push.Mode != "noop" {
		return c, fmt.Errorf("PUBLIC_FRONTEND_URL is required for external integrations")
	}
	c.EncryptionKey = os.Getenv("PROVIDER_TOKEN_ENCRYPTION_KEY")
	if c.EncryptionKey != "" {
		key, decodeErr := base64.StdEncoding.DecodeString(c.EncryptionKey)
		if decodeErr != nil || len(key) != 32 || c.EncryptionKey == os.Getenv("JWT_SECRET") || string(key) == os.Getenv("JWT_SECRET") {
			return c, fmt.Errorf("PROVIDER_TOKEN_ENCRYPTION_KEY must be an independent base64 AES-256 key")
		}
	}
	if (c.Push.Mode != "noop" || c.Calendar.Mode != "noop") && c.EncryptionKey == "" {
		return c, fmt.Errorf("push/calendar require PROVIDER_TOKEN_ENCRYPTION_KEY")
	}
	for key, target := range map[string]*string{"CALENDAR_OAUTH_AUTH_URL": &c.OAuthAuthURL, "CALENDAR_OAUTH_TOKEN_URL": &c.OAuthTokenURL, "CALENDAR_OAUTH_REVOKE_URL": &c.OAuthRevokeURL, "CALENDAR_OAUTH_REDIRECT_URL": &c.OAuthRedirectURL} {
		*target = os.Getenv(key)
		if *target != "" {
			if err := stageSevenURL(key, *target, local); err != nil {
				return c, err
			}
		}
	}
	c.OAuthClientID, c.OAuthClientSecret = os.Getenv("CALENDAR_OAUTH_CLIENT_ID"), os.Getenv("CALENDAR_OAUTH_CLIENT_SECRET")
	c.OAuthScopes = strings.Fields(os.Getenv("CALENDAR_OAUTH_SCOPES"))
	if c.OAuthAuthURL != "" || c.OAuthTokenURL != "" || c.OAuthRevokeURL != "" || c.OAuthRedirectURL != "" || c.OAuthClientID != "" || c.OAuthClientSecret != "" || len(c.OAuthScopes) != 0 {
		if c.Calendar.Mode != "http" || c.OAuthAuthURL == "" || c.OAuthTokenURL == "" || c.OAuthRevokeURL == "" || c.OAuthRedirectURL == "" || c.OAuthClientID == "" || c.OAuthClientSecret == "" || c.EncryptionKey == "" || len(c.OAuthScopes) == 0 {
			return c, fmt.Errorf("calendar OAuth configuration is incomplete")
		}
	}
	for _, part := range strings.Split(env("REMINDER_OFFSETS", "24h,15m"), ",") {
		duration, parseErr := time.ParseDuration(strings.TrimSpace(part))
		if parseErr != nil || duration < time.Minute || duration > 7*24*time.Hour || len(c.ReminderOffsets) >= 8 {
			return c, fmt.Errorf("REMINDER_OFFSETS requires 1..8 durations within 1m..168h")
		}
		for _, existing := range c.ReminderOffsets {
			if existing == duration {
				return c, fmt.Errorf("REMINDER_OFFSETS must not contain duplicates")
			}
		}
		c.ReminderOffsets = append(c.ReminderOffsets, duration)
	}
	integers := []struct {
		key                string
		target             *int
		initial, low, high int
	}{
		{"PRODUCT_WORKER_PORT", &c.Port, 8092, 1, 65535}, {"PRODUCT_DELIVERY_CONCURRENCY", &c.DeliveryWorkers, 2, 1, 8},
		{"PRODUCT_CALENDAR_CONCURRENCY", &c.CalendarWorkers, 1, 1, 8}, {"TRANSCRIPTION_CONCURRENCY", &c.STTWorkers, 1, 1, 8}, {"AI_JOB_CONCURRENCY", &c.AIWorkers, 1, 1, 8},
		{"TRANSCRIPTION_MAX_DURATION_SEC", &c.MaxDurationSec, 7200, 1, 86400}, {"TRANSCRIPTION_MAX_SEGMENTS", &c.MaxSegments, 10000, 1, 10000},
		{"AI_CHUNK_RUNES", &c.ChunkRunes, 12000, 512, 50000}, {"AI_MAX_CHUNKS", &c.MaxChunks, 256, 1, 512}, {"AI_CHUNK_CONCURRENCY", &c.AIConcurrency, 2, 1, 8},
		{"PRODUCT_MAX_REPROCESS", &c.MaxReprocess, 3, 0, 10}, {"PRODUCT_MAX_ATTEMPTS", &c.MaxAttempts, 5, 1, 10},
	}
	for _, field := range integers {
		raw := env(field.key, strconv.Itoa(field.initial))
		*field.target, err = strconv.Atoi(raw)
		if err != nil || *field.target < field.low || *field.target > field.high {
			return c, fmt.Errorf("%s must be %d..%d", field.key, field.low, field.high)
		}
	}
	for _, field := range []struct {
		key                string
		target             *int64
		initial, low, high int64
	}{
		{"TRANSCRIPTION_MAX_VIDEO_BYTES", &c.MaxVideoBytes, 1 << 30, 1 << 20, 8 << 30},
		{"TRANSCRIPTION_MAX_AUDIO_BYTES", &c.MaxAudioBytes, 256 << 20, 1 << 20, 2 << 30},
		{"TRANSCRIPTION_MAX_TEXT_BYTES", &c.MaxTranscriptBytes, 8 << 20, 1024, 32 << 20},
	} {
		*field.target, err = strconv.ParseInt(env(field.key, strconv.FormatInt(field.initial, 10)), 10, 64)
		if err != nil || *field.target < field.low || *field.target > field.high {
			return c, fmt.Errorf("%s is outside the allowed byte budget", field.key)
		}
	}
	for _, field := range []struct {
		key                string
		target             *time.Duration
		initial, low, high time.Duration
	}{
		{"PRODUCT_POLL_INTERVAL", &c.PollInterval, 5 * time.Second, 100 * time.Millisecond, time.Minute},
		{"PROVIDER_TIMEOUT", &c.ProviderTimeout, 15 * time.Second, time.Second, time.Minute},
		{"TRANSCRIPTION_TIMEOUT", &c.STTTimeout, 10 * time.Minute, time.Second, 30 * time.Minute},
		{"AI_SUMMARY_TIMEOUT", &c.AITimeout, 2 * time.Minute, time.Second, 30 * time.Minute},
		{"PRODUCT_REPROCESS_COOLDOWN", &c.ReprocessCooldown, 5 * time.Minute, time.Second, 24 * time.Hour},
	} {
		*field.target, err = time.ParseDuration(env(field.key, field.initial.String()))
		if err != nil || *field.target < field.low || *field.target > field.high {
			return c, fmt.Errorf("%s is outside the allowed timeout range", field.key)
		}
	}
	return c, nil
}

// stageSevenURL проверяет операторский URL без credentials, fragment и скрытой redirect-конфигурации.
// @args name — имя env для безопасной ошибки; raw — адрес; local — разрешение test HTTP.
// @return nil для абсолютного HTTPS URL либо локального HTTP, иначе ошибка конфигурации.
func stageSevenURL(name, raw string, local bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && !(local && u.Scheme == "http")) {
		return fmt.Errorf("%s must be an absolute HTTPS URL without credentials or fragment", name)
	}
	return nil
}

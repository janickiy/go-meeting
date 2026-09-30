package config

import (
	"fmt"
	"net"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config содержит настройки приложения из .env.
type Config struct {
	AppEnv             string
	APIPort            int
	WorkerPort         int
	WorkerInternalURL  string
	WorkerID           string
	StoragePath        string
	FFmpegPath         string
	WebRTCUDPPort      int
	WebRTCTCPPort      int
	WebRTCNATIPs       []string
	PostgresDSN        string
	RedisAddr          string
	RedisPassword      string
	RedisDB            int
	RabbitMQURL        string
	RabbitMQExchange   string
	RabbitMQQueue      string
	RabbitMQRoutingKey string
	RecordLockTTL      time.Duration
	RateLimitEnabled   bool
	RateLimitWindow    time.Duration
	RateLimit          RateLimitConfig
	MinIOEndpoint      string
	MinIOAccessKey     string
	MinIOSecretKey     string
	MinIOBucket        string
	MinIOUseSSL        bool
	MinIOPublicOrigin  string
}

// Load читает .env и переменные окружения.
// Параметры: нет.
// Возвращает: заполненный Config или ошибку некорректной настройки.
func Load() (Config, error) {
	_ = godotenv.Load()
	webRTCUDPPort := envInt("WEBRTC_UDP_PORT", 50000)

	cfg := Config{
		AppEnv:             env("APP_ENV", "local"),
		APIPort:            envInt("APP_PORT", 8085),
		WorkerPort:         envInt("WORKER_PORT", 8090),
		WorkerInternalURL:  env("WORKER_INTERNAL_URL", "http://worker:8090"),
		WorkerID:           env("WORKER_ID", "recorder-worker-local"),
		StoragePath:        env("STORAGE_PATH", "/storage"),
		FFmpegPath:         env("FFMPEG_PATH", "ffmpeg"),
		WebRTCUDPPort:      webRTCUDPPort,
		WebRTCTCPPort:      envInt("WEBRTC_TCP_PORT", webRTCUDPPort),
		WebRTCNATIPs:       envList("WEBRTC_NAT_IPS"),
		RedisAddr:          redisAddr(),
		RedisPassword:      env("REDIS_PASSWORD", ""),
		RedisDB:            envInt("REDIS_DB", 0),
		RabbitMQURL:        rabbitMQURL(),
		RabbitMQExchange:   env("RABBIT_MQ_EXCHANGE", "go-recorder.commands"),
		RabbitMQQueue:      env("RABBIT_MQ_QUEUE", "go-recorder.recording.commands"),
		RabbitMQRoutingKey: env("RABBIT_MQ_ROUTING_KEY", "record.commands"),
		RecordLockTTL:      envDuration("RECORD_LOCK_TTL", 6*time.Hour),
		RateLimitEnabled:   envBool("RATE_LIMIT_ENABLED", true),
		RateLimitWindow:    envDuration("RATE_LIMIT_WINDOW", time.Minute),
		RateLimit: RateLimitConfig{
			DefaultRPM:               envInt("RATE_LIMIT_DEFAULT_RPM", 240),
			RecordStartConferenceRPM: envInt("RATE_LIMIT_RECORD_START_CONFERENCE_RPM", 12),
			RecordStartIPRPM:         envInt("RATE_LIMIT_RECORD_START_IP_RPM", 40),
			RecordEndRecordRPM:       envInt("RATE_LIMIT_RECORD_END_RECORD_RPM", 40),
			RecordEndIPRPM:           envInt("RATE_LIMIT_RECORD_END_IP_RPM", 120),
			WebRTCOfferRecordRPM:     envInt("RATE_LIMIT_WEBRTC_OFFER_RECORD_RPM", 40),
			WebRTCOfferIPRPM:         envInt("RATE_LIMIT_WEBRTC_OFFER_IP_RPM", 120),
			RecordListIPRPM:          envInt("RATE_LIMIT_RECORD_LIST_IP_RPM", 240),
			RecordReadIPRPM:          envInt("RATE_LIMIT_RECORD_READ_IP_RPM", 480),
		},
		MinIOEndpoint:     env("MINIO_ENDPOINT", "minio:9000"),
		MinIOAccessKey:    env("MINIO_ROOT_USER", "go_recorder"),
		MinIOSecretKey:    env("MINIO_ROOT_PASSWORD", "go_recorder_pass"),
		MinIOBucket:       env("MINIO_BUCKET", "recordings"),
		MinIOUseSSL:       envBool("MINIO_USE_SSL", false),
		MinIOPublicOrigin: env("MINIO_PUBLIC_ENDPOINT", "localhost:9000"),
	}
	cfg.PostgresDSN = postgresDSN()
	if cfg.MinIOBucket == "" {
		return Config{}, fmt.Errorf("MINIO_BUCKET is required")
	}

	return cfg, nil
}

// RateLimitConfig содержит лимиты запросов API за одно окно.
type RateLimitConfig struct {
	DefaultRPM               int
	RecordStartConferenceRPM int
	RecordStartIPRPM         int
	RecordEndRecordRPM       int
	RecordEndIPRPM           int
	WebRTCOfferRecordRPM     int
	WebRTCOfferIPRPM         int
	RecordListIPRPM          int
	RecordReadIPRPM          int
}

// IsLocal проверяет, что приложение запущено в локальном окружении.
// Параметры: нет.
// Возвращает: true для APP_ENV=local/dev/debug.
func (c Config) IsLocal() bool {
	value := strings.ToLower(c.AppEnv)
	return value == "local" || value == "dev" || value == "debug"
}

func postgresDSN() string {
	host := env("POSTGRES_HOST", "postgres")
	port := env("POSTGRES_PORT", "5432")
	db := env("POSTGRES_DB", "go_recorder")
	user := env("POSTGRES_USER", "go_recorder")
	password := env("POSTGRES_PASSWORD", "go_recorder_pass")
	sslmode := env("POSTGRES_SSLMODE", "disable")

	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC", host, port, user, password, db, sslmode)
}

func redisAddr() string {
	host := env("REDIS_HOST", "redis")
	port := env("REDIS_PORT", "6379")

	return fmt.Sprintf("%s:%s", host, port)
}

func rabbitMQURL() string {
	if dsn := env("RABBIT_MQ_DSN", ""); dsn != "" {
		return dsn
	}
	user := env("RABBIT_MQ_USER", "go_recorder")
	password := env("RABBIT_MQ_PASSWORD", "go_recorder_pass")
	host := env("RABBIT_MQ_HOST", "rabbitmq")
	port := env("RABBIT_MQ_PORT", "5672")
	vhost := env("RABBIT_MQ_VHOST", "/")
	dsn := neturl.URL{
		Scheme:  "amqp",
		User:    neturl.UserPassword(user, password),
		Host:    net.JoinHostPort(host, port),
		Path:    "/" + vhost,
		RawPath: "/" + neturl.PathEscape(vhost),
	}

	return dsn.String()
}

func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}

	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err == nil {
		return parsed
	}
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return time.Duration(seconds) * time.Second
}

func envList(key string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}

	return result
}

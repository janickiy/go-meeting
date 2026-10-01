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

// Config собирает общие параметры окружения для запуска API и воркеров.
// Состав:
//   - AppEnv: значение AppEnv типа string, используемое согласно назначению этой операции.
//   - APIPort: значение APIPort типа int, используемое согласно назначению этой операции.
//   - WorkerPort: значение WorkerPort типа int, используемое согласно назначению этой операции.
//   - WorkerInternalURL: значение WorkerInternalURL типа string, используемое согласно назначению этой операции.
//   - WorkerID: идентификатор воркера-владельца операции.
//   - StoragePath: корневой каталог локального хранения артефактов записи.
//   - FFmpegPath: значение FFmpegPath типа string, используемое согласно назначению этой операции.
//   - WebRTCUDPPort: значение WebRTCUDPPort типа int, используемое согласно назначению этой операции.
//   - WebRTCTCPPort: значение WebRTCTCPPort типа int, используемое согласно назначению этой операции.
//   - WebRTCNATIPs: набор значений WebRTCNATIPs для последовательной или пакетной обработки.
//   - PostgresDSN: значение PostgresDSN типа string, используемое согласно назначению этой операции.
//   - RedisAddr: значение RedisAddr типа string, используемое согласно назначению этой операции.
//   - RedisPassword: значение RedisPassword типа string, используемое согласно назначению этой операции.
//   - RedisDB: значение RedisDB типа int, используемое согласно назначению этой операции.
//   - RabbitMQURL: значение RabbitMQURL типа string, используемое согласно назначению этой операции.
//   - RabbitMQExchange: значение RabbitMQExchange типа string, используемое согласно назначению этой операции.
//   - RabbitMQQueue: значение RabbitMQQueue типа string, используемое согласно назначению этой операции.
//   - RabbitMQRoutingKey: значение RabbitMQRoutingKey типа string, используемое согласно назначению этой операции.
//   - RecordLockTTL: значение RecordLockTTL типа time.Duration, используемое согласно назначению этой операции.
//   - RateLimitEnabled: логический признак RateLimitEnabled, управляющий соответствующей веткой обработки.
//   - RateLimitWindow: значение RateLimitWindow типа time.Duration, используемое согласно назначению этой операции.
//   - RateLimit: значение RateLimit типа RateLimitConfig, используемое согласно назначению этой операции.
//   - MinIOEndpoint: значение MinIOEndpoint типа string, используемое согласно назначению этой операции.
//   - MinIOAccessKey: значение MinIOAccessKey типа string, используемое согласно назначению этой операции.
//   - MinIOSecretKey: значение MinIOSecretKey типа string, используемое согласно назначению этой операции.
//   - MinIOBucket: значение MinIOBucket типа string, используемое согласно назначению этой операции.
//   - MinIOUseSSL: логический признак MinIOUseSSL, управляющий соответствующей веткой обработки.
//   - MinIOPublicOrigin: значение MinIOPublicOrigin типа string, используемое согласно назначению этой операции.
//   - JWTSecret: значение JWTSecret типа string, используемое согласно назначению этой операции.
//   - TrustedProxies: набор значений TrustedProxies для последовательной или пакетной обработки.
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
	JWTSecret          string
	TrustedProxies     []string
	Operations         OperationsConfig
}

// Load читает .env и переменные окружения.
// @parameters: нет.
// @return заполненный Config или ошибку некорректной настройки.
func Load() (Config, error) {
	_ = godotenv.Load()
	if err := validateTypedEnvironment(); err != nil {
		return Config{}, err
	}
	webRTCUDPPort := envInt("WEBRTC_UDP_PORT", 50000)

	cfg := Config{
		AppEnv:             env("APP_ENV", "local"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		TrustedProxies:     envList("HTTP_TRUSTED_PROXIES"),
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
			AuthLoginIPRPM:           envInt("RATE_LIMIT_AUTH_LOGIN_IP_RPM", 10),
			AuthRegisterIPRPM:        envInt("RATE_LIMIT_AUTH_REGISTER_IP_RPM", 5),
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
	var err error
	cfg.Operations, err = loadOperations(cfg.IsLocal() || cfg.AppEnv == "test")
	if err != nil {
		return Config{}, err
	}
	if err := validateProduction(cfg); err != nil {
		return Config{}, err
	}
	if cfg.MinIOBucket == "" {
		return Config{}, fmt.Errorf("MINIO_BUCKET is required")
	}

	return cfg, nil
}

// RateLimitConfig задаёт правила и режим обработки ограничения частоты запросов.
// Состав:
//   - DefaultRPM: значение DefaultRPM типа int, используемое согласно назначению этой операции.
//   - AuthLoginIPRPM: значение AuthLoginIPRPM типа int, используемое согласно назначению этой операции.
//   - AuthRegisterIPRPM: значение AuthRegisterIPRPM типа int, используемое согласно назначению этой операции.
//   - RecordStartConferenceRPM: значение RecordStartConferenceRPM типа int, используемое согласно назначению этой операции.
//   - RecordStartIPRPM: значение RecordStartIPRPM типа int, используемое согласно назначению этой операции.
//   - RecordEndRecordRPM: значение RecordEndRecordRPM типа int, используемое согласно назначению этой операции.
//   - RecordEndIPRPM: значение RecordEndIPRPM типа int, используемое согласно назначению этой операции.
//   - WebRTCOfferRecordRPM: значение WebRTCOfferRecordRPM типа int, используемое согласно назначению этой операции.
//   - WebRTCOfferIPRPM: значение WebRTCOfferIPRPM типа int, используемое согласно назначению этой операции.
//   - RecordListIPRPM: значение RecordListIPRPM типа int, используемое согласно назначению этой операции.
//   - RecordReadIPRPM: значение RecordReadIPRPM типа int, используемое согласно назначению этой операции.
type RateLimitConfig struct {
	DefaultRPM               int
	AuthLoginIPRPM           int
	AuthRegisterIPRPM        int
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
// @parameters: нет.
// @return true для APP_ENV=local/dev/debug.
func (c Config) IsLocal() bool {
	value := strings.ToLower(c.AppEnv)
	return value == "local" || value == "dev" || value == "debug"
}

// postgresDSN собирает строку подключения PostgreSQL из настроек окружения.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func postgresDSN() string {
	host := env("POSTGRES_HOST", "postgres")
	port := env("POSTGRES_PORT", "5432")
	db := env("POSTGRES_DB", "go_recorder")
	user := env("POSTGRES_USER", "go_recorder")
	password := env("POSTGRES_PASSWORD", "go_recorder_pass")
	sslmode := env("POSTGRES_SSLMODE", "disable")

	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC", host, port, user, password, db, sslmode)
}

// redisAddr собирает сетевой адрес Redis из настроек окружения.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func redisAddr() string {
	host := env("REDIS_HOST", "redis")
	port := env("REDIS_PORT", "6379")

	return fmt.Sprintf("%s:%s", host, port)
}

// rabbitMQURL собирает адрес подключения RabbitMQ из настроек окружения.
//
// @return:
//   - результат 1 (string): адрес разрешённого чтения или целевого ресурса.
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

// env читает строковую переменную окружения и применяет запасное значение при её отсутствии.
//
// @parameters:
//   - key (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
//   - fallback (string): значение, используемое при отсутствии входного параметра.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

// envInt читает целочисленный параметр окружения и проверяет его формат.
//
// @parameters:
//   - key (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
//   - fallback (int): значение, используемое при отсутствии входного параметра.
//
// @return:
//   - результат 1 (int): значение, подготовленное операцией для вызывающей стороны.
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

// envBool читает логический параметр окружения и проверяет допустимый формат.
//
// @parameters:
//   - key (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
//   - fallback (bool): значение, используемое при отсутствии входного параметра.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}

	return value == "1" || value == "true" || value == "yes" || value == "on"
}

// envDuration читает длительность из окружения и проверяет её формат.
//
// @parameters:
//   - key (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
//   - fallback (time.Duration): значение, используемое при отсутствии входного параметра.
//
// @return:
//   - результат 1 (time.Duration): значение, подготовленное операцией для вызывающей стороны.
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

// envList разбирает список значений переменной окружения.
//
// @parameters:
//   - key (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
//
// @return:
//   - результат 1 ([]string): собранные элементы результата; состав ограничивается параметрами операции.
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

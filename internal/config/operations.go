package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// OperationsConfig задаёт эксплуатационные границы одного процесса.
// ProbeTimeout ограничивает проверку зависимости, ProbeInterval объединяет частые
// обращения к readiness, ShutdownTimeout ограничивает завершение работы.
// DBMaxOpen/DBMaxIdle/DBLifetime управляют пулом БД; HTTPBodyBytes ограничивает JSON.
// InternalSecret защищает recorder HTTP, MetricsSecret — Prometheus; PprofPort=0
// отключает диагностику, иначе она слушает только loopback внутри контейнера.
type OperationsConfig struct {
	ProbeTimeout, ProbeInterval, ShutdownTimeout time.Duration
	DBMaxOpen, DBMaxIdle                         int
	DBLifetime                                   time.Duration
	HTTPBodyBytes                                int64
	InternalSecret, MetricsSecret                string
	PprofPort                                    int
	DiskMinBytes                                 uint64
	HTTPReadTimeout                              time.Duration
}

// loadOperations читает пределы из окружения и отвергает опасные значения.
// Аргумент local разрешает только локальную производную внутреннего секрета;
// результат содержит проверенную конфигурацию либо ошибку без значения секрета.
func loadOperations(local bool) (OperationsConfig, error) {
	c := OperationsConfig{
		ProbeTimeout: envDuration("HEALTH_PROBE_TIMEOUT", time.Second), ProbeInterval: envDuration("HEALTH_PROBE_INTERVAL", 2*time.Second),
		ShutdownTimeout: envDuration("SHUTDOWN_TIMEOUT", 20*time.Second),
		DBMaxOpen:       envInt("DB_MAX_OPEN", 20), DBMaxIdle: envInt("DB_MAX_IDLE", 10), DBLifetime: envDuration("DB_CONNECTION_LIFETIME", 30*time.Minute),
		HTTPBodyBytes: int64(envInt("HTTP_MAX_BODY_BYTES", 1<<20)), InternalSecret: os.Getenv("WORKER_INTERNAL_SECRET"),
		MetricsSecret: os.Getenv("METRICS_SECRET"), PprofPort: envInt("PPROF_PORT", 0),
		DiskMinBytes:    uint64(envInt("RECORDING_DISK_RESERVE_MIB", 256)) << 20,
		HTTPReadTimeout: envDuration("HTTP_READ_TIMEOUT", 60*time.Second),
	}
	if local && c.InternalSecret == "" && len(os.Getenv("JWT_SECRET")) >= 32 {
		c.InternalSecret = mediaDerivedSecret(os.Getenv("JWT_SECRET"), "recorder-internal-v1")
	}
	if c.ProbeTimeout < 50*time.Millisecond || c.ProbeTimeout > 5*time.Second || c.ProbeInterval < time.Second || c.ProbeInterval > time.Minute ||
		c.ShutdownTimeout < time.Second || c.ShutdownTimeout > 2*time.Minute || c.DBMaxOpen < 1 || c.DBMaxOpen > 500 || c.DBMaxIdle < 0 || c.DBMaxIdle > c.DBMaxOpen ||
		c.DBLifetime < time.Minute || c.DBLifetime > 24*time.Hour || c.HTTPBodyBytes < 65536 || c.HTTPBodyBytes > 16<<20 || c.PprofPort < 0 || c.PprofPort > 65535 {
		return c, fmt.Errorf("invalid operations limits or timeouts")
	}
	if c.DiskMinBytes > 100<<30 {
		return c, fmt.Errorf("RECORDING_DISK_RESERVE_MIB must be 0..102400")
	}
	if c.HTTPReadTimeout < time.Second || c.HTTPReadTimeout > 2*time.Minute {
		return c, fmt.Errorf("HTTP_READ_TIMEOUT must be 1s..2m")
	}
	if !local && (len(c.InternalSecret) < 32 || len(c.MetricsSecret) < 32) {
		return c, fmt.Errorf("production requires WORKER_INTERNAL_SECRET and METRICS_SECRET (32+ bytes)")
	}
	return c, nil
}

// validateProduction проверяет настройки при запуске production, не раскрывая
// секреты в ошибках. cfg содержит уже загруженные адреса, ключи и ограничения.
// Возвращает nil для допустимой конфигурации или причину отказа запуска.
func validateProduction(cfg Config) error {
	if cfg.IsLocal() || cfg.AppEnv == "test" {
		return nil
	}
	if cfg.AppEnv != "production" && cfg.AppEnv != "prod" {
		return fmt.Errorf("APP_ENV must be local, dev, debug, test, prod or production")
	}
	seen := map[string]bool{}
	for _, key := range []string{"JWT_SECRET", "MEDIA_TICKET_SECRET", "MEDIA_INTERNAL_SECRET", "WORKER_INTERNAL_SECRET", "METRICS_SECRET", "TURN_SHARED_SECRET"} {
		value := os.Getenv(key)
		if len(value) < 32 || strings.Contains(strings.ToLower(value), "replace") || seen[value] {
			return fmt.Errorf("%s requires a dedicated production secret", key)
		}
		seen[value] = true
	}
	if os.Getenv("TURN_URLS") == "" {
		return fmt.Errorf("production requires TURN_URLS")
	}
	for _, key := range []string{"POSTGRES_USER", "POSTGRES_DB", "MINIO_ROOT_USER", "RABBIT_MQ_USER", "MEDIA_NAT_IPS"} {
		if os.Getenv(key) == "" {
			return fmt.Errorf("production requires %s", key)
		}
	}
	if os.Getenv("GIN_MODE") != "release" || !cfg.RateLimitEnabled {
		return fmt.Errorf("production requires GIN_MODE=release and rate limits")
	}
	for _, key := range []string{"POSTGRES_PASSWORD", "RABBIT_MQ_PASSWORD", "MINIO_ROOT_PASSWORD", "REDIS_PASSWORD"} {
		value := os.Getenv(key)
		if len(value) < 16 || strings.Contains(value, "go_recorder") || strings.Contains(strings.ToLower(value), "replace") {
			return fmt.Errorf("%s requires a non-default production password", key)
		}
	}
	origins := envList("WS_ALLOWED_ORIGINS")
	if len(origins) == 0 {
		return fmt.Errorf("production requires explicit WS_ALLOWED_ORIGINS")
	}
	for _, origin := range origins {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("production WS_ALLOWED_ORIGINS must use HTTPS")
		}
	}
	u, err := url.Parse(cfg.MinIOPublicOrigin)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("production MINIO_PUBLIC_ENDPOINT must use HTTPS")
	}
	return nil
}

// validateTypedEnvironment предотвращает молчаливое применение значения по
// умолчанию при опечатке в числовом, логическом или временном параметре env.
// Аргументов нет; ошибка содержит только имя параметра, а не его значение.
func validateTypedEnvironment() error {
	for _, key := range []string{"HTTP_READ_TIMEOUT", "DB_QUERY_TIMEOUT"} {
		if raw := os.Getenv(key); raw != "" {
			if _, err := time.ParseDuration(raw); err != nil {
				return fmt.Errorf("%s must be a duration", key)
			}
		}
	}
	if raw := os.Getenv("RECORDING_DISK_RESERVE_MIB"); raw != "" {
		if _, err := strconv.Atoi(raw); err != nil {
			return fmt.Errorf("RECORDING_DISK_RESERVE_MIB must be an integer")
		}
	}
	ints := strings.Fields("APP_PORT APP_HOST_PORT WORKER_PORT WEBRTC_UDP_PORT WEBRTC_TCP_PORT WS_QUEUE_SIZE WS_MAX_MESSAGE_BYTES WS_MAX_OUTBOUND_BYTES WS_MAX_SDP_BYTES WS_MAX_ICE_BYTES WS_MESSAGES_PER_SECOND WS_MESSAGE_BURST WS_MAX_CONNECTIONS MEDIA_HTTP_PORT MEDIA_MAX_PEERS MEDIA_MAX_ROOMS MEDIA_MAX_PUBLISHED_TRACKS MEDIA_MAX_AUDIO_TRACKS MEDIA_MAX_VIDEO_TRACKS MEDIA_MAX_SCREEN_SHARERS MEDIA_EGRESS_QUEUE_SIZE MEDIA_VIDEO_MAX_WIDTH MEDIA_VIDEO_MAX_HEIGHT MEDIA_VIDEO_MAX_FPS MEDIA_UDP_PORT MEDIA_UDP_MIN_PORT MEDIA_UDP_MAX_PORT MEDIA_TCP_PORT DB_MAX_OPEN DB_MAX_IDLE HTTP_MAX_BODY_BYTES PPROF_PORT RECORDING_WIDTH RECORDING_HEIGHT RECORDING_FPS RECORDING_FFMPEG_CONCURRENCY RECORDING_MAX_ACTIVE RECORDING_MAX_MIB")
	for _, key := range ints {
		if v := os.Getenv(key); v != "" {
			if _, e := strconv.Atoi(v); e != nil {
				return fmt.Errorf("%s must be an integer", key)
			}
		}
	}
	for _, key := range strings.Fields("HEALTH_PROBE_TIMEOUT HEALTH_PROBE_INTERVAL SHUTDOWN_TIMEOUT DB_CONNECTION_LIFETIME TURN_CREDENTIAL_TTL WS_PING_INTERVAL WS_PONG_TIMEOUT WS_WRITE_TIMEOUT WS_SESSION_TTL WS_TICKET_TTL MEDIA_OPERATION_TIMEOUT MEDIA_HEARTBEAT_INTERVAL MEDIA_WORKER_TTL MEDIA_OWNERSHIP_TTL MEDIA_NEGOTIATION_TIMEOUT RECORDING_LEASE_TTL RECORDING_POLL_INTERVAL RECORDING_MAX_DURATION") {
		if v := os.Getenv(key); v != "" {
			if _, e := time.ParseDuration(v); e != nil {
				if _, e = strconv.Atoi(v); e != nil {
					return fmt.Errorf("%s must be a duration", key)
				}
			}
		}
	}
	for _, key := range strings.Fields("RATE_LIMIT_ENABLED MINIO_USE_SSL RECORDING_KEEP_LOCAL") {
		if v := strings.ToLower(os.Getenv(key)); v != "" && !strings.Contains("|true|false|1|0|yes|no|on|off|", "|"+v+"|") {
			return fmt.Errorf("%s must be a boolean", key)
		}
	}
	return nil
}

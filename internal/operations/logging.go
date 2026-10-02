package operations

import (
	"log/slog"
	"os"
	"strings"
)

// LogWriter переводит старые log.Logger в единый JSON-формат службы.
// Не хранит сообщения; секретные env-значения удаляются перед выводом.
type LogWriter struct{}

// Write выводит одну ограниченную запись. p содержит сообщение старого логгера;
// возвращает исходную длину и nil, сохраняя контракт io.Writer.
func (LogWriter) Write(p []byte) (int, error) {
	message := string(p)
	for _, key := range []string{"JWT_SECRET", "MEDIA_TICKET_SECRET", "MEDIA_INTERNAL_SECRET", "WORKER_INTERNAL_SECRET", "METRICS_SECRET", "TURN_SHARED_SECRET", "POSTGRES_PASSWORD", "REDIS_PASSWORD", "RABBIT_MQ_PASSWORD", "MINIO_ROOT_PASSWORD", "PROVIDER_TOKEN_ENCRYPTION_KEY", "EMAIL_PROVIDER_TOKEN", "PUSH_PROVIDER_TOKEN", "CALENDAR_PROVIDER_TOKEN", "STT_PROVIDER_TOKEN", "AI_PROVIDER_TOKEN", "CALENDAR_OAUTH_CLIENT_SECRET"} {
		if secret := os.Getenv(key); len(secret) >= 8 {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	if len(message) > 4096 {
		message = message[:4096] + " [truncated]"
	}
	slog.Info(strings.TrimSpace(message), "event_type", "component.log")
	return len(p), nil
}

// Пакет captions управляет изолированным живым распознаванием и восстановлением сохранённых финальных реплик.
package captions

import (
	"context"
	domain "github.com/janickiy/go-recorder/internal/domain/captions"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"io"
	"time"
)

// Lease привязывает вспомогательный процесс к текущей версии настройки конференции.
type Lease struct {
	domain.State
	Token     string
	EnabledAt time.Time
	Attempts  int
}

// Repository отделяет авторизацию и проверку владения в SQL от обработки звука.
type Repository interface {
	Read(context.Context, string, string) (domain.State, error)
	Set(context.Context, string, string, bool, string) (domain.State, error)
	Finals(context.Context, string, string, int64, int) ([]domain.Caption, error)
	Claim(context.Context, bool, int, int) (Lease, error)
	Renew(context.Context, Lease) (bool, error)
	Finish(context.Context, Lease, string) error
	Status(context.Context, Lease, string) error
	SaveFinal(context.Context, Lease, domain.Caption, int) (domain.Caption, error)
	Speaker(context.Context, string, string) (string, error)
	Observe(context.Context, Lease, string, int64, int64) error
}

// AudioTap открывает защищённый поток SFU, не используя RabbitMQ для отдельных кадров.
type AudioTap interface {
	Open(context.Context, string, string) (io.ReadCloser, error)
}

// Decoder преобразует Opus вне SFU в ограниченные PCM-порции.
type Decoder interface {
	Decode(context.Context, media.EgressTrack, <-chan media.EgressFrame, func([]byte) error) error
}

// Events публикует небольшие результаты через существующий транспорт реального времени.
type Events interface {
	Publish(context.Context, realtime.Bus) error
}

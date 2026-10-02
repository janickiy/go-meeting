package config

import (
	"fmt"
	"time"
)

// CompositeConfig задаёт ограничения общей записи, устойчивых фрагментов и обработки FFmpeg.
//
//	@params
//	 - Width: ширина видеокадра или области в пикселях.
//	 - Height: высота видеокадра или области в пикселях.
//	 - FPS: значение FPS типа int, используемое согласно назначению этой операции.
//	 - Concurrency: значение Concurrency типа int, используемое согласно назначению этой операции.
//	 - MaxActive: значение MaxActive типа int, используемое согласно назначению этой операции.
//	 - MaxBytes: значение MaxBytes типа int64, используемое согласно назначению этой операции.
//	 - LeaseTTL: значение LeaseTTL типа time.Duration, используемое согласно назначению этой операции.
//	 - PollInterval: значение PollInterval типа time.Duration, используемое согласно назначению этой операции.
//	 - MaxDuration: значение MaxDuration типа time.Duration, используемое согласно назначению этой операции.
//	 - KeepLocal: логический признак KeepLocal, управляющий соответствующей веткой обработки.
type CompositeConfig struct {
	Width        int
	Height       int
	FPS          int
	Concurrency  int
	MaxActive    int
	MaxBytes     int64
	LeaseTTL     time.Duration
	PollInterval time.Duration
	MaxDuration  time.Duration
	KeepLocal    bool
}

// LoadComposite читает и проверяет настройки общей записи, фрагментов и FFmpeg.
//
// @return:
//   - результат 1 (CompositeConfig): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func LoadComposite() (CompositeConfig, error) {
	c := CompositeConfig{Width: envInt("RECORDING_WIDTH", 1280), Height: envInt("RECORDING_HEIGHT", 720), FPS: envInt("RECORDING_FPS", 30), Concurrency: envInt("RECORDING_FFMPEG_CONCURRENCY", 1), MaxActive: envInt("RECORDING_MAX_ACTIVE", 2), MaxBytes: int64(envInt("RECORDING_MAX_MIB", 10240)) << 20, LeaseTTL: envDuration("RECORDING_LEASE_TTL", 20*time.Second), PollInterval: envDuration("RECORDING_POLL_INTERVAL", time.Second), MaxDuration: envDuration("RECORDING_MAX_DURATION", 4*time.Hour), KeepLocal: envBool("RECORDING_KEEP_LOCAL", false)}
	if c.Width < 320 || c.Width > 1920 || c.Width%2 != 0 || c.Height < 240 || c.Height > 1080 || c.Height%2 != 0 || c.FPS < 10 || c.FPS > 30 || c.Concurrency < 1 || c.Concurrency > 4 || c.MaxActive < 1 || c.MaxActive > 8 || c.MaxBytes < 1<<20 || c.MaxBytes > 100<<30 || c.LeaseTTL < 10*time.Second || c.LeaseTTL > time.Minute || c.PollInterval < 100*time.Millisecond || c.PollInterval > c.LeaseTTL/4 || c.MaxDuration < time.Minute || c.MaxDuration > 24*time.Hour {
		return CompositeConfig{}, fmt.Errorf("invalid composite recording limits")
	}
	return c, nil
}

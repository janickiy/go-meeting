package operations

import (
	"context"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

// ValidateStorage проверяет существование, права записи и запас диска в path.
// minimum — обязательный запас в байтах; пробный файл удаляется сразу.
func ValidateStorage(path string, minimum uint64) error {
	if path == "" {
		return fmt.Errorf("STORAGE_PATH must not be empty")
	}
	if err := os.MkdirAll(path, 0750); err != nil {
		return fmt.Errorf("storage directory unavailable")
	}
	file, err := os.CreateTemp(path, ".readiness-")
	if err != nil {
		return fmt.Errorf("storage is not writable")
	}
	_ = file.Close()
	_ = os.Remove(file.Name())
	return DiskCheck(path, minimum)(context.Background())
}

// DiskCheck создаёт readiness-проверку локального диска. path — каталог записи,
// minimum — минимальный запас; контекст проверяется до короткого Statfs.
func DiskCheck(path string, minimum uint64) Check {
	return func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var stat unix.Statfs_t
		if err := unix.Statfs(path, &stat); err != nil {
			return fmt.Errorf("storage stat failed")
		}
		free := uint64(stat.Bavail) * uint64(stat.Bsize)
		State("disk_free_bytes", float64(free))
		if free < minimum {
			Event("disk_low")
			return fmt.Errorf("storage free space below reserve")
		}
		return nil
	}
}

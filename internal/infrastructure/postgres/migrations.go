package postgres

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// RunMigrations применяет SQL-файлы из database/migrations.
// Параметры:
// - db: GORM-подключение.
// - dir: директория миграций.
// Возвращает: ошибку чтения или выполнения SQL.
func RunMigrations(db *gorm.DB, dir string) error {
	matches, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return fmt.Errorf("glob migrations: %w", err)
	}
	sort.Strings(matches)

	for _, path := range matches {
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", path, err)
		}
		sql := strings.TrimSpace(string(content))
		if sql == "" {
			continue
		}
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("apply migration %s: %w", filepath.Base(path), err)
		}
	}

	return nil
}

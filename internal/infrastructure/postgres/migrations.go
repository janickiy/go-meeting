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
	return db.Transaction(func(tx *gorm.DB) error {
		// Multiple API instances must not race CREATE TABLE or DROP/CREATE TRIGGER.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(748239105)").Error; err != nil {
			return err
		}
		for _, path := range matches {
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read migration %s: %w", path, err)
			}
			sql := strings.TrimSpace(string(content))
			if sql == "" {
				continue
			}
			if err := tx.Exec(sql).Error; err != nil {
				return fmt.Errorf("apply migration %s: %w", filepath.Base(path), err)
			}
		}

		return nil
	})
}

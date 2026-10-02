package local

import (
	"fmt"
	"os"
	"path/filepath"
)

// RemoveEmptyTrees удаляет переданные директории только если они пустые.
// @args
// - paths: пути к директориям, внутри которых нужно удалить пустые поддиректории.
// @return ошибку чтения или удаления директории.
func RemoveEmptyTrees(paths ...string) error {
	for _, path := range paths {
		if err := removeEmptyTree(path); err != nil {
			return err
		}
	}

	return nil
}

// removeEmptyTree рекурсивно проверяет каталог и удаляет его только после опустошения дочерних каталогов.
//
// @args
//   - path (string): путь к локальному файлу или каталогу операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func removeEmptyTree(path string) error {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read local storage dir %s: %w", path, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := removeEmptyTree(filepath.Join(path, entry.Name())); err != nil {
			return err
		}
	}
	entries, err = os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read local storage dir %s: %w", path, err)
	}
	if len(entries) > 0 {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove empty local storage dir %s: %w", path, err)
	}

	return nil
}

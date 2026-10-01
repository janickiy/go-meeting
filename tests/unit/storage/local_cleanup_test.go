package storage_test

import (
	"os"
	"path/filepath"
	"testing"

	localstorage "github.com/janickiy/go-recorder/internal/infrastructure/storage/local"
)

// TestRemoveEmptyTreesDeletesEmptyRecordAndTmpDirs проверяет сценарий «удаление пустой Trees Deletes пустой запись и Tmp Dirs», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestRemoveEmptyTreesDeletesEmptyRecordAndTmpDirs(t *testing.T) {
	root := t.TempDir()
	recordDir := filepath.Join(root, "records", "record-id", "nested")
	tmpDir := filepath.Join(root, "tmp", "record-id")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatalf("create record dir: %v", err)
	}
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatalf("create tmp dir: %v", err)
	}

	if err := localstorage.RemoveEmptyTrees(filepath.Join(root, "records", "record-id"), tmpDir); err != nil {
		t.Fatalf("RemoveEmptyTrees() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "records", "record-id")); !os.IsNotExist(err) {
		t.Fatalf("record dir exists after cleanup, err=%v", err)
	}
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Fatalf("tmp dir exists after cleanup, err=%v", err)
	}
}

// TestRemoveEmptyTreesKeepsNonEmptyRecordDir проверяет сценарий «удаление пустой Trees Keeps не пустой запись Dir», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestRemoveEmptyTreesKeepsNonEmptyRecordDir(t *testing.T) {
	root := t.TempDir()
	recordDir := filepath.Join(root, "records", "record-id")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatalf("create record dir: %v", err)
	}
	filePath := filepath.Join(recordDir, "segment_000001.mkv")
	if err := os.WriteFile(filePath, []byte("media"), 0o644); err != nil {
		t.Fatalf("write segment: %v", err)
	}

	if err := localstorage.RemoveEmptyTrees(recordDir); err != nil {
		t.Fatalf("RemoveEmptyTrees() error = %v", err)
	}

	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("segment file was removed, err=%v", err)
	}
}

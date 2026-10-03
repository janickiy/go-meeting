package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestManifestIntegrity проверяет контрольные суммы, запрет обхода путей и ссылок до обращения к целевому бакету.
// @args t — контекст проверок недоверенного файла резервной копии.
func TestManifestIntegrity(t *testing.T) {
	for _, mode := range []string{"valid", "corrupt", "path", "symlink", "duplicate", "version"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			payload := []byte("synthetic-recording-fixture")
			hash := sha256.Sum256(payload)
			name := strings.Repeat("a", 64) + ".bin"
			m := manifest{SchemaVersion: 1, Bucket: "test-backup", Objects: []object{{Key: "records/test/final.mp4", File: name, SHA256: hex.EncodeToString(hash[:]), Size: int64(len(payload))}}}
			if err := os.WriteFile(filepath.Join(dir, name), payload, 0600); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "corrupt":
				m.Objects[0].SHA256 = strings.Repeat("0", 64)
			case "path":
				m.Objects[0].File = "../secret"
			case "symlink":
				link := strings.Repeat("b", 64) + ".bin"
				if err := os.Symlink(filepath.Join(dir, name), filepath.Join(dir, link)); err != nil {
					t.Fatal(err)
				}
				m.Objects[0].File = link
			case "duplicate":
				m.Objects = append(m.Objects, m.Objects[0])
			case "version":
				m.SchemaVersion = 999
			}
			data, _ := json.Marshal(m)
			if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			_, err := readManifest(dir)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("integrity check did not enforce %s", mode)
			}
		})
	}
}

package releasecontracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// archiveFixture создаёт исключительно синтетический пакет для проверки отказов gate.
// Это не Docker-архив и не доказательство безопасности настоящего релиза.
// @args t — контекст теста; change — изменение одного отчёта до расчёта checksum.
// @return путь к синтетическому манифесту и каталогу пакета.
func archiveFixture(t *testing.T, change func(map[string]any)) (string, string) {
	t.Helper()
	dir := t.TempDir()
	archive := []byte("not a deployable image archive: security contract fixture")
	writeFixture(t, filepath.Join(dir, "images.tar"), archive)
	images := map[string]string{}
	evidence := map[string]any{}
	services := strings.Fields("api media-worker worker product-worker live-worker frontend minio postgres redis rabbitmq coturn proxy prometheus")
	for _, service := range services {
		id := "sha256:" + digest([]byte(service))
		images[service] = id
		report := map[string]any{
			"SchemaVersion": 2,
			"Metadata":      map[string]any{"ImageID": id, "ImageConfig": map[string]any{"architecture": "amd64"}},
			"Results":       []any{map[string]any{"Vulnerabilities": []any{}}},
		}
		if service == "api" && change != nil {
			change(report)
		}
		scan, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		sbom := []byte(`{"bomFormat":"CycloneDX","specVersion":"1.6"}`)
		writeFixture(t, filepath.Join(dir, service+".scan.json"), scan)
		writeFixture(t, filepath.Join(dir, service+".sbom.json"), sbom)
		evidence[service] = map[string]string{"scanSHA256": digest(scan), "sbomSHA256": digest(sbom)}
	}
	manifest, err := json.Marshal(map[string]any{"platform": "linux/amd64", "archiveSHA256": digest(archive), "images": images, "securityEvidence": evidence})
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, "release.json")
	writeFixture(t, manifestPath, manifest)
	return manifestPath, dir
}

// digest считает SHA-256 тестовых байтов в формате, используемом release scripts.
// @args data — байты синтетического артефакта; @return шестнадцатеричный checksum.
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// writeFixture сохраняет тестовый файл только внутри t.TempDir с закрытыми правами.
// @args t — контекст; name — абсолютный путь тестового файла; data — его байты.
func writeFixture(t *testing.T, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
}

// checkFixture вызывает настоящий Bash gate без Docker и без сети.
// @args t — контекст теста; manifest — синтетический манифест.
// @return диагностический вывод и ошибка завершения gate.
func checkFixture(t *testing.T, manifest string) (string, error) {
	t.Helper()
	common, err := filepath.Abs("../../scripts/release/common.sh")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("bash", "-c", `source "$1"; RELEASE_MANIFEST="$2"; select_release_services hardened-shared-host; verify_archive_security`, "test", common, manifest)
	output, err := command.CombinedOutput()
	return string(output), err
}

// TestArchiveSecurityContract проверяет, что новый способ доставки не обходит scan.
// @args t — контекст набора позитивных и негативных контрактных сценариев.
func TestArchiveSecurityContract(t *testing.T) {
	cases := []struct {
		name   string
		change func(map[string]any)
		tamper string
		valid  bool
	}{
		{name: "intact", valid: true},
		{name: "high", change: func(r map[string]any) {
			r["Results"] = []any{map[string]any{"Vulnerabilities": []any{map[string]any{"Severity": "HIGH"}}}}
		}},
		{name: "critical", change: func(r map[string]any) {
			r["Results"] = []any{map[string]any{"Vulnerabilities": []any{map[string]any{"Severity": "CRITICAL"}}}}
		}},
		{name: "wrong-image", change: func(r map[string]any) {
			r["Metadata"].(map[string]any)["ImageID"] = "sha256:" + strings.Repeat("0", 64)
		}},
		{name: "wrong-architecture", change: func(r map[string]any) {
			r["Metadata"].(map[string]any)["ImageConfig"].(map[string]any)["architecture"] = "arm64"
		}},
		{name: "missing-results", change: func(r map[string]any) { delete(r, "Results") }},
		{name: "tampered-image-archive", tamper: "images.tar"},
		{name: "tampered-scan", tamper: "api.scan.json"},
		{name: "tampered-sbom", tamper: "api.sbom.json"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			manifest, dir := archiveFixture(t, item.change)
			if item.tamper != "" {
				writeFixture(t, filepath.Join(dir, item.tamper), []byte("tampered"))
			}
			output, err := checkFixture(t, manifest)
			if (err == nil) != item.valid {
				t.Fatalf("unexpected gate result: %v, %s", err, output)
			}
		})
	}
}

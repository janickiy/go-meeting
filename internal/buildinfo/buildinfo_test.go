package buildinfo

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPublicBuildMetadata отклоняет секреты, пути и инъекции в сборочных значениях.
// @args t — контекст проверки JSON публичной версии.
func TestPublicBuildMetadata(t *testing.T) {
	oldVersion, oldCommit, oldTime := Version, Commit, BuildTime
	t.Cleanup(func() { Version, Commit, BuildTime = oldVersion, oldCommit, oldTime })
	Version, Commit, BuildTime = "v1.2.3", strings.Repeat("a", 40), "2026-10-03T14:00:00Z"
	info := Current()
	if info.Version != Version || info.Commit != Commit || info.BuildTime != BuildTime {
		t.Fatal(info)
	}
	Version, Commit, BuildTime = "secret/path\n", "Bearer sensitive", "private/path"
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/version", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "sensitive") || strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), "secret") {
		t.Fatal(w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("build metadata cached")
	}
}

// Пакет buildinfo раскрывает только проверенные публичные сведения об артефакте.
package buildinfo

import (
	"encoding/json"
	"net/http"
	"runtime/debug"
	"time"
)

// Version, Commit и BuildTime задаются при сборке через -ldflags -X.
var Version, Commit, BuildTime string

// Info содержит версию релиза, SHA исходников и время сборки без окружения и путей.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
}

// ValidVersion допускает короткий идентификатор релиза без управляющих символов.
// @args value — версия или идентификатор сборки.
// @return true только для безопасного значения длиной до 64 байт.
func ValidVersion(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for index, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || index > 0 && (char == '.' || char == '_' || char == '-' || char == '+') {
			continue
		}
		return false
	}
	return true
}

// Current проверяет внедрённые метаданные; при отсутствии SHA использует VCS stamp.
// @return безопасные поля, где dev/unknown обозначают неполную локальную сборку.
func Current() Info {
	i := Info{Version: "dev", Commit: "unknown", BuildTime: "unknown"}
	if ValidVersion(Version) {
		i.Version = Version
	}
	commit := Commit
	if info, ok := debug.ReadBuildInfo(); ok && commit == "" {
		for _, value := range info.Settings {
			if value.Key == "vcs.revision" {
				commit = value.Value
			}
		}
	}
	valid := len(commit) == 40 || len(commit) == 64
	for _, char := range commit {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			valid = false
		}
	}
	if valid {
		i.Commit = commit
	}
	if stamp, err := time.Parse(time.RFC3339, BuildTime); err == nil {
		i.BuildTime = stamp.UTC().Format(time.RFC3339)
	}
	return i
}

// Handler возвращает сведения о сборке без секретов, флагов компилятора и внутренних адресов.
// @args w — ответ; r — запрос, содержимое которого не используется.
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_ = json.NewEncoder(w).Encode(Current())
}

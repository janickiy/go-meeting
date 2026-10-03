// Пакет telemetry принимает обезличенные технические ошибки браузера.
package telemetry

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

const maxBodyBytes = 8192

var versionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,79}$`)
var assetPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+-[A-Za-z0-9_-]{8,}\.js$`)

// Frame указывает только позицию в собранном ресурсе, без URL, имени функции и текста ошибки.
// @params: File — имя ресурса; Line и Column — положительные координаты в нём.
type Frame struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Report содержит разрешённый набор диагностических полей без пользовательского содержимого.
// @params: Version — версия интерфейса; Route — шаблон маршрута; Code — класс ошибки;
// Browser — семейство браузера; Stack — до пяти позиций в собранных ресурсах.
type Report struct {
	Version string  `json:"version"`
	Route   string  `json:"route"`
	Code    string  `json:"code"`
	Browser string  `json:"browser"`
	Stack   []Frame `json:"stack"`
}

// Handler принимает только строго проверенные отчёты и считает ошибки без меток высокой кардинальности.
// @params: enabled включает приём; logger получает обезличенные поля; errors считает ограниченные категории.
type Handler struct {
	enabled bool
	logger  *slog.Logger
	errors  *prometheus.CounterVec
}

// NewHandler создаёт обработчик и регистрирует отдельный счётчик клиентских ошибок.
// @args enabled — явное включение; logger — журнал приложения; registry — реестр API.
// @return обработчик, по умолчанию закрывающий маршрут при выключенной настройке.
func NewHandler(enabled bool, logger *slog.Logger, registry prometheus.Registerer) *Handler {
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "recorder_client_errors_total",
		Help: "Validated browser errors by fixed code, browser family and route template.",
	}, []string{"code", "browser", "route"})
	registry.MustRegister(counter)
	return &Handler{enabled: enabled, logger: logger, errors: counter}
}

// Receive проверяет размер, формат и все поля до журналирования или обновления метрик.
// @args c — HTTP-запрос без обязательной авторизации, чтобы учитывать ошибки экрана входа.
func (h *Handler) Receive(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if !h.enabled {
		c.Status(http.StatusNotFound)
		return
	}
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		c.Status(http.StatusUnsupportedMediaType)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var report Report
	if err := decoder.Decode(&report); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.Status(http.StatusRequestEntityTooLarge)
		} else {
			c.Status(http.StatusBadRequest)
		}
		return
	}
	if decoder.Decode(new(any)) != io.EOF || !validReport(report) {
		c.Status(http.StatusBadRequest)
		return
	}
	h.errors.WithLabelValues(report.Code, report.Browser, report.Route).Inc()
	h.logger.Warn("client error", "version", report.Version, "route", report.Route,
		"code", report.Code, "browser", report.Browser, "stack", report.Stack)
	c.Status(http.StatusAccepted)
}

// validReport запрещает свободный текст, полные маршруты, адреса и неизвестные категории.
// @args report — ещё не доверенный результат разбора JSON.
// @return true только для ограниченного технического отчёта.
func validReport(report Report) bool {
	if !versionPattern.MatchString(report.Version) || len(report.Stack) > 5 {
		return false
	}
	switch report.Code {
	case "uncaught_error", "unhandled_rejection", "react_render_error":
	default:
		return false
	}
	switch report.Browser {
	case "chromium", "firefox", "safari", "edge", "other":
	default:
		return false
	}
	switch report.Route {
	case "/", "/login", "/register", "/register/success", "/app", "/meetings", "/meetings/new",
		"/meetings/:id", "/meetings/:id/join", "/conferences", "/conferences/new", "/conferences/:id",
		"/conferences/:id/join", "/i/:code", "/app/settings", "/settings",
		"/app/settings/calendar/:provider/callback", "/app/recordings", "/history", "/history/:id",
		"/notifications", "/app/search", "/search", "/admin", "/calendar", "/analytics", "unknown":
	default:
		return false
	}
	for _, frame := range report.Stack {
		if len(frame.File) > 160 || !assetPattern.MatchString(frame.File) || frame.Line < 1 || frame.Line > 10_000_000 || frame.Column < 1 || frame.Column > 10_000_000 {
			return false
		}
	}
	return true
}

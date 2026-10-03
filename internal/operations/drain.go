package operations

import (
	"encoding/json"
	"net/http"
)

// ConfigureDrain задаёт остановку приёма работ и подсчёт активности до запуска HTTP.
// @args begin — идемпотентное закрытие приёма; active — точное число незавершённых работ.
func (r *Runtime) ConfigureDrain(begin func(), active func() int) {
	r.beginDrain, r.activeWork = begin, active
}

// DrainStatus запускает или читает необратимое завершение работы с внутренним секретом.
// @args w — ответ; req — GET либо POST с METRICS_SECRET, без JSON и идентификаторов работ.
func (r *Runtime) DrainStatus(w http.ResponseWriter, req *http.Request) {
	if !Authorized(req, r.Config.MetricsSecret) {
		http.NotFound(w, req)
		return
	}
	if req.Method == http.MethodPost {
		r.Drain()
		if r.beginDrain != nil {
			r.beginDrain()
		}
	}
	active := int(r.inflight.Load())
	if r.activeWork != nil {
		active += r.activeWork()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Draining bool `json:"draining"`
		Active   int  `json:"active"`
		Ready    bool `json:"ready"`
	}{r.draining.Load(), active, r.Ready()})
}

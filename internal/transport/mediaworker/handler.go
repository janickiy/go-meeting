// Package mediaworker exposes only the authenticated, internal signaling plane.
// RTP never passes through these HTTP handlers.
package mediaworker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/pion/webrtc/v4"
)

// Engine задаёт контракт зависимого компонента Engine в защищённом управлении медиа-комнатой; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - Join: операция Join с контрактом, описанным у метода.
//   - Offer: операция SDP-предложение с контрактом, описанным у метода.
//   - Ready: операция готовность с контрактом, описанным у метода.
//   - ICE: операция ICE с контрактом, описанным у метода.
//   - Unpublish: операция Unpublish с контрактом, описанным у метода.
//   - Leave: операция Leave с контрактом, описанным у метода.
//   - LeaveConnection: операция Leave Connection с контрактом, описанным у метода.
//   - PeerBinding: операция Peer Binding с контрактом, описанным у метода.
//   - Bindings: операция Bindings с контрактом, описанным у метода.
//   - Tracks: операция дорожки с контрактом, описанным у метода.
//   - CloseConference: операция закрытие конференция с контрактом, описанным у метода.
//   - Shutdown: операция завершение с контрактом, описанным у метода.
type Engine interface {
	// Join создаёт или восстанавливает членство участника, учитывая приглашение, состояние встречи и зал ожидания.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (media.Binding): проверенная идентичность медиа-подключения, назначенная сервером.
	//
	// @return:
	//   - результат 1 (media.PeerView): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Join(context.Context, media.Binding) (media.PeerView, error)
	// Offer обрабатывает или передаёт SDP-предложение действующего WebRTC-подключения.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (string): идентификатор связанного ресурса, заданного параметром negotiationID.
	//   - аргумент 4 (string): исходные байты JSON, пакета или сериализованного значения.
	//
	// @return:
	//   - результат 1 (webrtc.SessionDescription): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Offer(context.Context, string, string, string) (webrtc.SessionDescription, error)
	// Ready принимает подтверждение установки SDP-ответа клиентом и разрешает дорожки только для текущего согласования.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (string): идентификатор связанного ресурса, заданного параметром negotiationID.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Ready(context.Context, string, string) error
	// ICE передаёт проверенного кандидата ICE действующему медиа-соединению.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (*webrtc.ICECandidateInit): проверенный кандидат ICE для WebRTC-соединения.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	ICE(context.Context, string, *webrtc.ICECandidateInit) error
	// Unpublish останавливает публикацию указанного медиа-источника.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (string): идентификатор связанного ресурса, заданного параметром trackID.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Unpublish(context.Context, string, string) error
	// Leave фиксирует выход участника, сохраняя историю членства и состояние допуска.
	//
	// @args
	//   - аргумент 1 (context.Context): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Leave(context.Context, string) error
	// LeaveConnection отключает только указанное физическое медиа-соединение.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор физического медиа-соединения.
	LeaveConnection(context.Context, string)
	// PeerBinding возвращает серверную идентичность медиа-пира по физическому соединению.
	//
	// @args
	//   - аргумент 1 (string): идентификатор обрабатываемого ресурса.
	//
	// @return:
	//   - результат 1 (media.Binding): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (bool): признак выполнения проверяемого условия или изменения состояния.
	PeerBinding(string) (media.Binding, bool)
	// Bindings возвращает серверные идентичности подключений в комнате.
	//
	//
	// @return:
	//   - результат 1 ([]media.Binding): собранные элементы результата; состав ограничивается параметрами операции.
	Bindings() []media.Binding
	// Tracks возвращает снимок доступных опубликованных дорожек комнаты.
	//
	// @args
	//   - аргумент 1 (string): идентификатор обрабатываемого ресурса.
	//
	// @return:
	//   - результат 1 ([]media.Track): собранные элементы результата; состав ограничивается параметрами операции.
	Tracks(string) []media.Track
	// CloseConference закрывает все медиа-подключения и ресурсы конкретной конференции.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	CloseConference(context.Context, string) error
	// Shutdown останавливает менеджер и ожидает завершения принадлежащих ему ресурсов.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Shutdown(context.Context) error
}

// Registry задаёт контракт зависимого компонента Registry в защищённом управлении медиа-комнатой; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params:
//   - RegisterWorker: операция Register воркер с контрактом, описанным у метода.
//   - GetOwner: операция получение владелец с контрактом, описанным у метода.
//   - Renew: операция Renew с контрактом, описанным у метода.
//   - Release: операция освобождение с контрактом, описанным у метода.
//   - RemoveWorker: операция удаление воркер с контрактом, описанным у метода.
type Registry interface {
	// RegisterWorker сохраняет сведения и срок присутствия доступного медиа-воркера.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (media.Worker): значение worker типа media.Worker, используемое согласно назначению этой операции.
	//   - аргумент 3 (time.Duration): срок жизни сохраняемого значения или выданного разрешения.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	RegisterWorker(context.Context, media.Worker, time.Duration) error
	// GetOwner читает актуального владельца медиа-комнаты и его версию владения.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//
	// @return:
	//   - результат 1 (media.Route): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	GetOwner(context.Context, string) (media.Route, error)
	// Renew продлевает владение только при совпадении идентичности текущего владельца.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 3 (media.Route): адрес и версия действующего владельца медиа-комнаты.
	//   - аргумент 4 (time.Duration): срок жизни сохраняемого значения или выданного разрешения.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Renew(context.Context, string, media.Route, time.Duration) error
	// Release освобождает ресурс только при совпадении сохранённого владельца или токена.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 3 (media.Route): адрес и версия действующего владельца медиа-комнаты.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Release(context.Context, string, media.Route) error
	// RemoveWorker удаляет присутствие воркера из реестра.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор воркера-владельца операции.
	//   - аргумент 3 (string): адрес конечной точки вызываемого сервиса.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	RemoveWorker(context.Context, string, string) error
}

// Sessions задаёт контракт зависимого компонента Sessions в защищённом управлении медиа-комнатой; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params:
//   - Get: операция получение с контрактом, описанным у метода.
type Sessions interface {
	// Get читает состояние ресурсов компонента для дальнейшей обработки или ответа.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//
	// @return:
	//   - результат 1 (realtime.Session): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Get(context.Context, string) (realtime.Session, error)
}

// Tickets задаёт контракт зависимого компонента Tickets в защищённом управлении медиа-комнатой; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params:
//   - Verify: операция Verify с контрактом, описанным у метода.
type Tickets interface {
	// Verify проверяет подпись, срок и содержимое переданного разрешения согласно контракту сервиса.
	//
	// @args
	//   - аргумент 1 (string): исходные байты JSON, пакета или сериализованного значения.
	//
	// @return:
	//   - результат 1 (media.Binding): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (media.Route): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Verify(string) (media.Binding, media.Route, error)
}

// offerCache сохраняет результат согласования SDP для безопасной повторной обработки предложения.
// @params:
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - id: идентификатор обрабатываемого ресурса.
//   - hash: сохранённый хеш пароля либо контрольная сумма данных.
//   - answer: значение answer типа string, используемое согласно назначению этой операции.
type offerCache struct {
	mu     sync.Mutex
	id     string
	hash   [32]byte
	answer string
}

// roomGate сериализует управляющие операции одной медиа-комнаты.
// @params:
//   - token: подписанный токен или токен владения, который необходимо проверить.
//   - refs: значение refs типа int, используемое согласно назначению этой операции.
type roomGate struct {
	token chan struct{}
	refs  int
}

// roomLease хранит локальное подтверждение владения комнатой и срок его действия.
// @params:
//   - route: адрес и версия действующего владельца медиа-комнаты.
//   - deadline: момент, после которого ожидание или действие прекращается.
type roomLease struct {
	route    media.Route
	deadline time.Time
}

// Handler связывает транспортный запрос с прикладным сценарием, проверкой входных данных и формированием ответа.
// @params:
//   - cfg: проверенные настройки соответствующего компонента.
//   - registry: распределённый реестр воркеров и владения комнатами.
//   - sessions: хранилище и авторизация физических сессий подключения.
//   - tickets: сервис выпуска и проверки ограниченных билетов подключения.
//   - engine: значение engine типа Engine, используемое согласно назначению этой операции.
//   - logger: значение logger типа *slog.Logger, используемое согласно назначению этой операции.
//   - metrics: операция metrics с контрактом, описанным у метода.
//   - ready: значение ready типа atomic.Bool, используемое согласно назначению этой операции.
//   - gate: канал «gate» для передачи данных или завершения ожидания.
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - leases: индекс значений leases для поиска и согласования состояния.
//   - offers: индекс значений offers для поиска и согласования состояния.
//   - roomGates: индекс значений roomGates для поиска и согласования состояния.
//   - workerDeadline: временная отметка workerDeadline; указатель допускает отсутствие значения.
//   - fencing: логический признак fencing, управляющий соответствующей веткой обработки.
//   - closing: логический признак closing, управляющий соответствующей веткой обработки.
type Handler struct {
	cfg            config.MediaConfig
	registry       Registry
	sessions       Sessions
	tickets        Tickets
	engine         Engine
	logger         *slog.Logger
	metrics        func() any
	ready          atomic.Bool
	gate           chan struct{}
	mu             sync.Mutex
	leases         map[string]roomLease
	offers         map[string]*offerCache
	roomGates      map[string]*roomGate
	workerDeadline time.Time
	fencing        bool
	closing        bool
}

// NewHandler создаёт и связывает зависимости компонента Handler, используемого в защищённом управлении медиа-комнатой.
//
// @args
//   - cfg (config.MediaConfig): проверенные настройки соответствующего компонента.
//   - registry (Registry): распределённый реестр воркеров и владения комнатами.
//   - sessions (Sessions): хранилище и авторизация физических сессий подключения.
//   - tickets (Tickets): сервис выпуска и проверки ограниченных билетов подключения.
//   - engine (Engine): значение engine типа Engine, используемое согласно назначению этой операции.
//   - metrics (func() any): вызываемый обработчик «metrics» с контрактом, указанным в типе.
//   - logger (*slog.Logger): значение logger типа *slog.Logger, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (*Handler): созданный компонент с переданными зависимостями.
func NewHandler(cfg config.MediaConfig, registry Registry, sessions Sessions, tickets Tickets, engine Engine, metrics func() any, logger *slog.Logger) *Handler {
	return &Handler{cfg: cfg, registry: registry, sessions: sessions, tickets: tickets, engine: engine, metrics: metrics, logger: logger, gate: make(chan struct{}, 128), leases: make(map[string]roomLease), offers: make(map[string]*offerCache), roomGates: make(map[string]*roomGate)}
}

// lockRoom сериализует управляющую операцию по одной комнате и возвращает освобождение блокировки.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (func()): функция продолжения или освобождения ресурса с указанным контрактом.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (h *Handler) lockRoom(ctx context.Context, id string) (func(), error) {
	if ctx.Err() != nil {
		return nil, media.ErrUnavailable
	}
	h.mu.Lock()
	gate := h.roomGates[id]
	if gate == nil {
		gate = &roomGate{token: make(chan struct{}, 1)}
		gate.token <- struct{}{}
		h.roomGates[id] = gate
	}
	gate.refs++
	h.mu.Unlock()
	// Вложенный обработчик выполняет выделенный шаг обработки в защищённом управлении медиа-комнатой, используя состояние окружающей функции.
	// Синхронизирует доступ к разделяемому состоянию блокировкой.
	//
	releaseRef := func() {
		h.mu.Lock()
		gate.refs--
		if gate.refs == 0 {
			delete(h.roomGates, id)
		}
		h.mu.Unlock()
	}
	select {
	case <-ctx.Done():
		releaseRef()
		return nil, media.ErrUnavailable
	case <-gate.token:
		// Вложенный обработчик выполняет выделенный шаг обработки в защищённом управлении медиа-комнатой, используя состояние окружающей функции.
		//
		return func() { gate.token <- struct{}{}; releaseRef() }, nil
	}
}

// Routes собирает внутренний HTTP-маршрутизатор медиа-воркера с авторизацией и проверками готовности.
//
// @return:
//   - результат 1 (http.Handler): значение, подготовленное операцией для вызывающей стороны.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", /* Вложенный обработчик выполняет выделенный шаг обработки в защищённом управлении медиа-комнатой, используя состояние окружающей функции.

		@args
		  - w (http.ResponseWriter): получатель HTTP-ответа.
		  - _ (*http.Request): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
		*/func(w http.ResponseWriter, _ *http.Request) {
			if !h.ready.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	mux.HandleFunc("GET /internal/media/metrics", h.auth( /* Вложенный обработчик выполняет выделенный шаг обработки в защищённом управлении медиа-комнатой, используя состояние окружающей функции.

		@args
		  - w (http.ResponseWriter): получатель HTTP-ответа.
		  - _ (*http.Request): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
		*/func(w http.ResponseWriter, _ *http.Request) { h.json(w, http.StatusOK, h.metrics()) }))
	for _, operation := range []string{"join", "offer", "ready", "ice", "leave", "unpublish", "policy", "close"} {
		op := operation
		mux.HandleFunc("POST /internal/media/"+op, h.auth( /* Вложенный обработчик выполняет выделенный шаг обработки в защищённом управлении медиа-комнатой, используя состояние окружающей функции.

			@args
			  - w (http.ResponseWriter): получатель HTTP-ответа.
			  - r (*http.Request): входящий HTTP-запрос.
			*/func(w http.ResponseWriter, r *http.Request) { h.command(w, r, op) }))
	}
	mux.HandleFunc("POST /internal/media/egress", h.auth(h.egress))
	return mux
}

// auth проверяет секрет защищённого внутреннего запроса API к медиа-воркеру.
//
// @args
//   - next (http.HandlerFunc): значение next типа http.HandlerFunc, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (http.HandlerFunc): значение, подготовленное операцией для вызывающей стороны.
func (h *Handler) auth(next http.HandlerFunc) http.HandlerFunc {
	// Вложенный обработчик выполняет выделенный шаг обработки в защищённом управлении медиа-комнатой, используя состояние окружающей функции.
	//
	// @args
	//   - w (http.ResponseWriter): получатель HTTP-ответа.
	//   - r (*http.Request): входящий HTTP-запрос.
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if provided == r.Header.Get("Authorization") || subtle.ConstantTimeCompare([]byte(provided), []byte(h.cfg.InternalSecret)) != 1 {
			h.fail(w, media.ErrUnauthorized)
			return
		}
		next(w, r)
	}
}

// strictJSON строго разбирает JSON-пакет и отвергает неизвестные поля и лишние данные.
//
// @args
//   - raw ([]byte): исходные байты JSON, пакета или сериализованного значения.
//   - target (any): целевой объект, участник или состояние операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func strictJSON(raw []byte, target any) error {
	if !utf8.Valid(raw) {
		return media.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return media.ErrInvalid
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return media.ErrInvalid
	}
	return nil
}

// validUUID проверяет корректность и ненулевое значение UUID.
//
// @args
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func validUUID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed != uuid.Nil && parsed.String() == id
}

// sameBinding сравнивает серверную идентичность двух медиа-подключений.
//
// @args
//   - a (media.Binding): значение a типа media.Binding, используемое согласно назначению этой операции.
//   - b (media.Binding): контекст измерения производительности теста.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func sameBinding(a, b media.Binding) bool {
	return a.ConferenceID == b.ConferenceID && a.ParticipantID == b.ParticipantID && a.SessionID == b.SessionID && a.ConnectionID == b.ConnectionID && a.UserID == b.UserID && a.AuthorizationExpiresAt.Equal(b.AuthorizationExpiresAt)
}

// sameRoute сравнивает владельца, адрес и версию двух маршрутов медиа-комнаты.
//
// @args
//   - a (media.Route): значение a типа media.Route, используемое согласно назначению этой операции.
//   - b (media.Route): контекст измерения производительности теста.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func sameRoute(a, b media.Route) bool {
	return a.WorkerID == b.WorkerID && a.Endpoint == b.Endpoint && a.LeaseID == b.LeaseID
}

// active проверяет наличие активного медиа-ресурса и его действующей серверной идентичности.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - binding (media.Binding): проверенная идентичность медиа-подключения, назначенная сервером.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (h *Handler) active(ctx context.Context, binding media.Binding) error {
	if !binding.AuthorizationExpiresAt.After(time.Now()) {
		return media.ErrUnauthorized
	}
	s, err := h.sessions.Get(ctx, binding.ConnectionID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, media.ErrUnauthorized) || errors.Is(err, media.ErrPeerNotFound) {
			return media.ErrUnauthorized
		}
		return media.ErrUnavailable
	}
	if s.ID != binding.SessionID || s.ConferenceID != binding.ConferenceID || s.ParticipantID != binding.ParticipantID || s.ConnectionID != binding.ConnectionID || s.UserID != binding.UserID || s.Status != "connected" {
		return media.ErrUnauthorized
	}
	return nil
}

// command разбирает и исполняет разрешённую внутреннюю команду управления медиа.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - w (http.ResponseWriter): получатель HTTP-ответа.
//   - r (*http.Request): входящий HTTP-запрос.
//   - operation (string): имя внутренней операции обработки.
func (h *Handler) command(w http.ResponseWriter, r *http.Request, operation string) {
	select {
	case h.gate <- struct{}{}:
		defer /* Вложенный обработчик выполняет выделенный шаг обработки в защищённом управлении медиа-комнатой, используя состояние окружающей функции.

		 */func() { <-h.gate }()
	default:
		h.fail(w, media.ErrLimit)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	raw, err := io.ReadAll(r.Body)
	var cmd media.Command
	if err != nil || strictJSON(raw, &cmd) != nil || !validUUID(cmd.RequestID) || r.Header.Get("X-Request-ID") != cmd.RequestID {
		h.fail(w, media.ErrInvalid)
		return
	}
	w.Header().Set("X-Request-ID", cmd.RequestID)
	if operation == "policy" || operation == "close" {
		h.serviceCommand(w, r, operation, cmd)
		return
	}
	for _, id := range []string{cmd.Binding.ConferenceID, cmd.Binding.ParticipantID, cmd.Binding.SessionID, cmd.Binding.ConnectionID, cmd.Binding.UserID, cmd.Route.LeaseID} {
		if !validUUID(id) {
			h.fail(w, media.ErrInvalid)
			return
		}
	}
	if cmd.Route.WorkerID != h.cfg.WorkerID || cmd.Route.Endpoint != h.cfg.WorkerInternalURL {
		h.fail(w, media.ErrOwnership)
		return
	}
	if operation != "join" && !validUUID(cmd.MediaPeerID) {
		h.fail(w, media.ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.OperationTimeout)
	defer cancel()
	if operation == "join" || operation == "leave" {
		unlock, err := h.lockRoom(ctx, cmd.Binding.ConferenceID)
		if err != nil {
			h.fail(w, err)
			return
		}
		defer unlock()
	}
	if operation == "leave" {
		// Expired authorization and lost ownership must not prevent cleanup.
		if binding, ok := h.engine.PeerBinding(cmd.MediaPeerID); ok && !sameBinding(binding, cmd.Binding) {
			h.fail(w, media.ErrUnauthorized)
			return
		}
		err = h.engine.Leave(ctx, cmd.MediaPeerID)
		h.mu.Lock()
		delete(h.offers, cmd.MediaPeerID)
		h.mu.Unlock()
		if err != nil {
			h.fail(w, err)
			return
		}
		// The periodic sweep releases an empty room under the same room gate.
		h.json(w, http.StatusOK, media.Result{MediaPeerID: cmd.MediaPeerID})
		return
	}
	if !h.ready.Load() {
		h.fail(w, media.ErrUnavailable)
		return
	}
	owner, err := h.registry.GetOwner(ctx, cmd.Binding.ConferenceID)
	if err != nil || !sameRoute(owner, cmd.Route) {
		h.fail(w, media.ErrOwnership)
		return
	}
	if err := h.active(ctx, cmd.Binding); err != nil {
		h.fail(w, err)
		return
	}
	if operation == "join" {
		if cmd.Policy != nil && (cmd.Policy.Version < 0 || cmd.Policy.Kicked) {
			h.fail(w, media.ErrPolicy)
			return
		}
		binding, route, err := h.tickets.Verify(cmd.Ticket)
		if err != nil || !sameBinding(binding, cmd.Binding) || !sameRoute(route, cmd.Route) {
			h.fail(w, media.ErrUnauthorized)
			return
		}
		renewedAt := time.Now()
		if err := h.registry.Renew(ctx, binding.ConferenceID, route, h.cfg.OwnershipTTL); err != nil {
			h.fail(w, media.ErrOwnership)
			return
		}
		// Reassignment to this same process must not reuse a stale room from an
		// older fencing lease. Join/leave for this room are serialized here.
		h.mu.Lock()
		old, existed := h.leases[binding.ConferenceID]
		h.mu.Unlock()
		if existed && !sameRoute(old.route, route) {
			_ = h.engine.CloseConference(ctx, binding.ConferenceID)
		}
		h.mu.Lock()
		admitted := !h.fencing && !h.closing && h.workerDeadline.After(time.Now()) && renewedAt.Add(h.cfg.OwnershipTTL).After(time.Now())
		if admitted {
			h.leases[binding.ConferenceID] = roomLease{route: route, deadline: renewedAt.Add(h.cfg.OwnershipTTL)}
		}
		h.mu.Unlock()
		if !admitted {
			h.fail(w, media.ErrUnavailable)
			return
		}
		// A legitimate leave/rejoin carries a newer persisted policy. Apply it
		// to an existing room before admission, then again for a newly made room.
		if cmd.Policy != nil {
			engine, ok := h.engine.(interface {
				SetPolicy(context.Context, string, string, media.ParticipantPolicy) error
			})
			if !ok {
				h.fail(w, media.ErrUnavailable)
				return
			}
			if err = engine.SetPolicy(ctx, binding.ConferenceID, binding.ParticipantID, *cmd.Policy); err != nil {
				h.fail(w, err)
				return
			}
		}
		peer, err := h.engine.Join(ctx, binding)
		if err != nil {
			h.fail(w, err)
			return
		}
		if cmd.Policy != nil {
			if policyEngine, ok := h.engine.(interface {
				SetPolicy(context.Context, string, string, media.ParticipantPolicy) error
			}); ok {
				if err = policyEngine.SetPolicy(ctx, binding.ConferenceID, binding.ParticipantID, *cmd.Policy); err != nil {
					_ = h.engine.Leave(ctx, peer.MediaPeerID)
					h.fail(w, err)
					return
				}
			} else {
				_ = h.engine.Leave(ctx, peer.MediaPeerID)
				h.fail(w, media.ErrUnavailable)
				return
			}
		}
		if !h.leaseValid(binding.ConferenceID, route) {
			_ = h.engine.Leave(ctx, peer.MediaPeerID)
			h.fail(w, media.ErrOwnership)
			return
		}
		h.logger.Info("media peer admitted", "conference_id", binding.ConferenceID, "participant_id", binding.ParticipantID, "session_id", binding.SessionID, "media_peer_id", peer.MediaPeerID, "worker_id", h.cfg.WorkerID, "request_id", cmd.RequestID)
		policy := cmd.Policy
		if provider, ok := h.engine.(interface {
			ParticipantPolicy(string, string) (media.ParticipantPolicy, bool)
		}); ok {
			if current, exists := provider.ParticipantPolicy(binding.ConferenceID, binding.ParticipantID); exists {
				policy = &current
			}
		}
		ice := h.cfg.TURN.ClientICE(h.cfg.ICE, time.Now())
		h.json(w, http.StatusOK, media.Result{MediaPeerID: peer.MediaPeerID, WorkerID: h.cfg.WorkerID, MaxPeers: h.cfg.MaxPeers, ICEServers: ice.ICEServers, ICETransportPolicy: ice.ICETransportPolicy, ICEExpiresAt: ice.ExpiresAt, Tracks: h.engine.Tracks(peer.MediaPeerID), Policy: policy, VideoCapture: media.VideoCaptureTarget{MaxWidth: h.cfg.VideoMaxWidth, MaxHeight: h.cfg.VideoMaxHeight, MaxFrameRate: h.cfg.VideoMaxFPS}})
		return
	}
	binding, ok := h.engine.PeerBinding(cmd.MediaPeerID)
	if !ok {
		h.fail(w, media.ErrPeerNotFound)
		return
	}
	if !sameBinding(binding, cmd.Binding) {
		h.fail(w, media.ErrUnauthorized)
		return
	}
	result := media.Result{MediaPeerID: cmd.MediaPeerID}
	switch operation {
	case "offer":
		if !validUUID(cmd.NegotiationID) || len(cmd.SDP) == 0 || len(cmd.SDP) > media.MaxSDPBytes {
			h.fail(w, media.ErrInvalid)
			return
		}
		if cmd.Policy != nil {
			policyEngine, ok := h.engine.(interface {
				SetPolicy(context.Context, string, string, media.ParticipantPolicy) error
			})
			if !ok {
				h.fail(w, media.ErrUnavailable)
				return
			}
			if err = policyEngine.SetPolicy(ctx, binding.ConferenceID, binding.ParticipantID, *cmd.Policy); err != nil {
				h.fail(w, err)
				return
			}
			if cmd.Policy.Kicked {
				h.fail(w, media.ErrPolicy)
				return
			}
		}
		h.mu.Lock()
		cache := h.offers[cmd.MediaPeerID]
		if cache == nil {
			cache = &offerCache{}
			h.offers[cmd.MediaPeerID] = cache
		}
		h.mu.Unlock()
		cache.mu.Lock()
		metadata, _ := json.Marshal(cmd.Publications)
		hash := sha256.Sum256(append(append([]byte(cmd.SDP), 0), metadata...))
		if cache.id == cmd.NegotiationID {
			if cache.hash != hash {
				err = media.ErrNegotiation
			} else {
				result.SDP = cache.answer
			}
		} else {
			var answer webrtc.SessionDescription
			if engine, ok := h.engine.(interface {
				OfferSources(context.Context, string, string, string, []media.Publication) (webrtc.SessionDescription, error)
			}); ok {
				answer, err = engine.OfferSources(ctx, cmd.MediaPeerID, cmd.NegotiationID, cmd.SDP, cmd.Publications)
			} else if cmd.Publications != nil {
				err = media.ErrInvalid
			} else {
				answer, err = h.engine.Offer(ctx, cmd.MediaPeerID, cmd.NegotiationID, cmd.SDP)
			}
			if err == nil {
				result.SDP = answer.SDP
				cache.id, cache.hash, cache.answer = cmd.NegotiationID, hash, answer.SDP
			}
		}
		cache.mu.Unlock()
		result.NegotiationID = cmd.NegotiationID
	case "ready":
		if !validUUID(cmd.NegotiationID) || cmd.SDP != "" || len(cmd.Candidate) != 0 || cmd.TrackID != "" {
			h.fail(w, media.ErrInvalid)
			return
		}
		h.mu.Lock()
		cache := h.offers[cmd.MediaPeerID]
		h.mu.Unlock()
		if cache == nil {
			h.fail(w, media.ErrNegotiation)
			return
		}
		cache.mu.Lock()
		if cache.id != cmd.NegotiationID {
			err = media.ErrNegotiation
		} else {
			err = h.engine.Ready(ctx, cmd.MediaPeerID, cmd.NegotiationID)
		}
		cache.mu.Unlock()
	case "ice":
		if len(cmd.Candidate) == 0 || len(cmd.Candidate) > media.MaxICEBytes {
			h.fail(w, media.ErrInvalid)
			return
		}
		var candidate *webrtc.ICECandidateInit
		if string(cmd.Candidate) != "null" {
			candidate = &webrtc.ICECandidateInit{}
			if strictJSON(cmd.Candidate, candidate) != nil {
				h.fail(w, media.ErrInvalid)
				return
			}
		}
		err = h.engine.ICE(ctx, cmd.MediaPeerID, candidate)
	case "unpublish":
		if !validUUID(cmd.TrackID) {
			h.fail(w, media.ErrInvalid)
			return
		}
		err = h.engine.Unpublish(ctx, cmd.MediaPeerID, cmd.TrackID)
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.json(w, http.StatusOK, result)
}

// fail фиксирует ошибочное завершение и запускает предусмотренную очистку ресурса.
//
// @args
//   - w (http.ResponseWriter): получатель HTTP-ответа.
//   - err (error): ошибка, которую необходимо классифицировать, сохранить или вернуть клиенту.
func (h *Handler) fail(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, media.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, media.ErrLimit):
		status = http.StatusTooManyRequests
	case errors.Is(err, media.ErrPeerNotFound):
		status = http.StatusNotFound
	case errors.Is(err, media.ErrOwnership), errors.Is(err, media.ErrNegotiation), errors.Is(err, media.ErrScreenConflict):
		status = http.StatusConflict
	case errors.Is(err, media.ErrPolicy):
		status = http.StatusForbidden
	case errors.Is(err, media.ErrUnavailable):
		status = http.StatusServiceUnavailable
	}
	h.json(w, status, map[string]string{"code": media.ErrorCode(err)})
}

// json сериализует данные и записывает JSON-ответ с заданным HTTP-статусом.
//
// @args
//   - w (http.ResponseWriter): получатель HTTP-ответа.
//   - status (int): состояние ресурса, ответа или фильтра выборки.
//   - data (any): полезная нагрузка события или байты обрабатываемого содержимого.
func (h *Handler) json(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// Start запускает обработку ресурсов компонента и подготавливает связанные ресурсы.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//
// @return:
//   - результат 1 (<-chan struct{}): канал данных или уведомления о завершении, принадлежащий жизненному циклу компонента.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (h *Handler) Start(ctx context.Context) (<-chan struct{}, error) {
	startedAt := time.Now()
	operation, cancel := context.WithTimeout(ctx, h.cfg.OperationTimeout)
	err := h.registry.RegisterWorker(operation, media.Worker{ID: h.cfg.WorkerID, Endpoint: h.cfg.WorkerInternalURL}, h.cfg.WorkerTTL)
	cancel()
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	h.workerDeadline = startedAt.Add(h.cfg.WorkerTTL)
	h.updateReadyLocked()
	h.mu.Unlock()
	done := make(chan struct{})
	var workers sync.WaitGroup
	// Вложенный обработчик выполняет выделенный шаг обработки в защищённом управлении медиа-комнатой, используя состояние окружающей функции.
	//
	// @args
	//   - interval (time.Duration): значение interval типа time.Duration, используемое согласно назначению этой операции.
	//   - tick (func(context.Context)): вызываемый обработчик «tick» с контрактом, указанным в типе.
	run := func(interval time.Duration, tick func(context.Context)) {
		workers.Add(1)
		go /* Вложенный обработчик выполняет выделенный шаг обработки в защищённом управлении медиа-комнатой, используя состояние окружающей функции.

		 */func() {
			defer workers.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					h.ready.Store(false)
					return
				case <-ticker.C:
					tick(ctx)
				}
			}
		}()
	}
	run(h.cfg.HeartbeatInterval, h.heartbeat)
	run(h.cfg.SessionCheckInterval, h.sweep)
	fenceInterval := min(h.cfg.WorkerTTL, h.cfg.OwnershipTTL) / 4
	fenceInterval = max(10*time.Millisecond, min(time.Second, fenceInterval))
	run(fenceInterval, h.watchdog)
	go /* Вложенный обработчик выполняет выделенный шаг обработки в защищённом управлении медиа-комнатой, используя состояние окружающей функции.

	 */func() { workers.Wait(); close(done) }()
	return done, nil
}

// updateReadyLocked обновляет признак готовности сервиса при удерживаемой блокировке его состояния.
func (h *Handler) updateReadyLocked() {
	now := time.Now()
	valid := !h.fencing && !h.closing && h.workerDeadline.After(now)
	for _, lease := range h.leases {
		if !lease.deadline.After(now) {
			valid = false
			break
		}
	}
	h.ready.Store(valid)
}

// leaseValid проверяет, принадлежит ли медиа-комната этому воркеру с действующей версией аренды.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - route (media.Route): адрес и версия действующего владельца медиа-комнаты.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func (h *Handler) leaseValid(conferenceID string, route media.Route) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	lease, ok := h.leases[conferenceID]
	return ok && !h.fencing && !h.closing && h.workerDeadline.After(time.Now()) && lease.deadline.After(time.Now()) && sameRoute(lease.route, route)
}

// watchdog проверяет сохранение авторизации и владения ресурсами и закрывает утратившие право ресурсы.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
func (h *Handler) watchdog(ctx context.Context) {
	h.mu.Lock()
	now := time.Now()
	expired := !h.fencing && !h.closing && !h.workerDeadline.After(now)
	for _, lease := range h.leases {
		if !lease.deadline.After(now) {
			expired = true
			break
		}
	}
	h.mu.Unlock()
	if expired {
		h.fence(ctx, "local_lease_expired")
	}
}

// fence прекращает действия компонента после потери действующего владения, чтобы устаревший воркер не продолжил медиа.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - reason (string): причина завершения, отказа или изменения состояния.
func (h *Handler) fence(ctx context.Context, reason string) {
	h.mu.Lock()
	if h.fencing || h.closing {
		h.mu.Unlock()
		return
	}
	if reason == "local_lease_expired" {
		now := time.Now()
		expired := !h.workerDeadline.After(now)
		for _, lease := range h.leases {
			if !lease.deadline.After(now) {
				expired = true
				break
			}
		}
		if !expired {
			h.mu.Unlock()
			return
		} // A concurrent renewal won the race.
	}
	h.fencing = true
	h.ready.Store(false)
	h.workerDeadline = time.Time{}
	rooms := make(map[string]bool)
	for id := range h.leases {
		rooms[id] = true
	}
	h.leases = make(map[string]roomLease)
	h.mu.Unlock()
	h.logger.Warn("media worker fenced", "worker_id", h.cfg.WorkerID, "event_type", reason)
	for _, binding := range h.engine.Bindings() {
		rooms[binding.ConferenceID] = true
	}
	var closed sync.WaitGroup
	for id := range rooms {
		closed.Add(1)
		go /* Вложенный обработчик выполняет выделенный шаг обработки в защищённом управлении медиа-комнатой, используя состояние окружающей функции.

		@args
		  - id (string): идентификатор обрабатываемого ресурса.
		*/func(id string) { defer closed.Done(); _ = h.engine.CloseConference(ctx, id) }(id)
	}
	closed.Wait()
	h.mu.Lock()
	h.offers = make(map[string]*offerCache)
	h.fencing = false
	// Only a successful post-fence heartbeat may make the worker ready again.
	h.ready.Store(false)
	h.mu.Unlock()
}

// heartbeat периодически продлевает активность компонента в распределённом реестре.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
func (h *Handler) heartbeat(ctx context.Context) {
	operation, cancel := context.WithTimeout(ctx, h.cfg.OperationTimeout)
	defer cancel()
	startedAt := time.Now()
	if err := h.registry.RegisterWorker(operation, media.Worker{ID: h.cfg.WorkerID, Endpoint: h.cfg.WorkerInternalURL}, h.cfg.WorkerTTL); err != nil {
		h.fence(operation, "registry_unavailable")
		return
	}
	h.mu.Lock()
	h.workerDeadline = startedAt.Add(h.cfg.WorkerTTL)
	leases := make(map[string]roomLease, len(h.leases))
	for id, lease := range h.leases {
		leases[id] = lease
	}
	h.mu.Unlock()
	for id, lease := range leases {
		renewedAt := time.Now()
		// Never place liveness renewal behind a slow join/leave room gate.
		if err := h.registry.Renew(operation, id, lease.route, h.cfg.OwnershipTTL); err != nil {
			if !errors.Is(err, media.ErrOwnership) {
				h.fence(operation, "registry_unavailable")
				return
			}
			unlock, err := h.lockRoom(operation, id)
			if err != nil {
				h.fence(operation, "lease_cleanup_timeout")
				return
			}
			h.mu.Lock()
			current, ok := h.leases[id]
			matches := ok && sameRoute(current.route, lease.route)
			if matches {
				delete(h.leases, id)
			}
			h.mu.Unlock()
			if matches {
				_ = h.engine.CloseConference(operation, id)
				h.logger.Warn("media room ownership lost", "conference_id", id, "worker_id", h.cfg.WorkerID)
			}
			unlock()
			continue
		}
		h.mu.Lock()
		if current, ok := h.leases[id]; ok && sameRoute(current.route, lease.route) {
			current.deadline = renewedAt.Add(h.cfg.OwnershipTTL)
			h.leases[id] = current
		}
		h.mu.Unlock()
	}
	h.releaseEmpty(operation)
	h.mu.Lock()
	h.updateReadyLocked()
	h.mu.Unlock()
}

// sweep периодически удаляет истёкшие сессии и освобождает утратившие доступ ресурсы.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
func (h *Handler) sweep(ctx context.Context) {
	operation, cancel := context.WithTimeout(ctx, h.cfg.OperationTimeout)
	defer cancel()
	for _, binding := range h.engine.Bindings() {
		if operation.Err() != nil {
			h.fence(operation, "session_sweep_timeout")
			return
		}
		err := h.active(operation, binding)
		if errors.Is(err, media.ErrUnavailable) {
			h.fence(operation, "session_store_unavailable")
			return
		}
		if err != nil {
			unlock, err := h.lockRoom(operation, binding.ConferenceID)
			if err != nil {
				h.fence(operation, "session_cleanup_timeout")
				return
			}
			h.engine.LeaveConnection(operation, binding.ConnectionID)
			unlock()
		}
	}
	h.releaseEmpty(operation)
}

// releaseEmpty освобождает владение комнатами, в которых больше нет активных подключений.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
func (h *Handler) releaseEmpty(ctx context.Context) {
	h.mu.Lock()
	leases := make(map[string]roomLease, len(h.leases))
	for id, lease := range h.leases {
		leases[id] = lease
	}
	for id := range h.offers {
		if _, ok := h.engine.PeerBinding(id); !ok {
			delete(h.offers, id)
		}
	}
	h.mu.Unlock()
	for id, lease := range leases {
		unlock, err := h.lockRoom(ctx, id)
		if err != nil {
			return
		}
		active := false
		if engine, ok := h.engine.(interface{ HasConference(string) bool }); ok {
			active = engine.HasConference(id)
		}
		for _, binding := range h.engine.Bindings() {
			if binding.ConferenceID == id {
				active = true
				break
			}
		}
		if !active {
			h.mu.Lock()
			if current, ok := h.leases[id]; ok && sameRoute(current.route, lease.route) {
				delete(h.leases, id)
			}
			h.mu.Unlock()
			_ = h.registry.Release(ctx, id, lease.route)
		}
		unlock()
	}
}

// Stop останавливает активную обработку ресурсов компонента и освобождает связанные ресурсы.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (h *Handler) Stop(ctx context.Context) error {
	h.mu.Lock()
	h.closing = true
	h.ready.Store(false)
	h.mu.Unlock()
	err := h.engine.Shutdown(ctx)
	h.releaseEmpty(ctx)
	_ = h.registry.RemoveWorker(ctx, h.cfg.WorkerID, h.cfg.WorkerInternalURL)
	return err
}

// Ready читает состояние аренды и draining без Redis-запроса. Результат true
// означает, что worker ещё вправе принимать новые медиа-команды.
func (h *Handler) Ready() bool { return h.ready.Load() }

package realtime

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	domain "github.com/janickiy/meet-space/internal/domain/realtime"
)

// Repository задаёт контракт зависимого компонента Repository в присутствии участников и доставке realtime-событий; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - Authorize: операция Authorize с контрактом, описанным у метода.
//   - Open: операция открытие с контрактом, описанным у метода.
//   - Close: операция закрытие с контрактом, описанным у метода.
//   - Stale: операция устаревший с контрактом, описанным у метода.
//   - Roster: операция Roster с контрактом, описанным у метода.
type Repository interface {
	// Authorize проверяет право пользователя участвовать в операции до работы с защищёнными ресурсами.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 3 (string): идентификатор пользователя, для которого выполняется операция.
	//
	// @return:
	//   - результат 1 (conferences.Participant): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Authorize(context.Context, string, string) (conferences.Participant, error)
	// Open создаёт историческое физическое соединение после проверки права участника.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (domain.Session): историческая физическая сессия или состояние текущего соединения.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Open(context.Context, domain.Session) error
	// Close закрывает принадлежащие компоненту ресурсы и завершает связанный жизненный цикл.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор физического медиа-соединения.
	//   - аргумент 3 (time.Time): временная отметка seen; указатель допускает отсутствие значения.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Close(context.Context, string, time.Time) error
	// Stale находит незакрытые исторические сессии старше контрольной границы.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (time.Time): временная отметка before; указатель допускает отсутствие значения.
	//   - аргумент 3 (string): непрозрачная граница продолжения предыдущей страницы.
	//
	// @return:
	//   - результат 1 ([]domain.Session): собранные элементы результата; состав ограничивается параметрами операции.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Stale(context.Context, time.Time, string) ([]domain.Session, error)
	// Roster читает конференцию и членства для авторизации и построения снимка комнаты.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//
	// @return:
	//   - результат 1 (conferences.Status): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 ([]conferences.Participant): собранные элементы результата; состав ограничивается параметрами операции.
	//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Roster(context.Context, string) (conferences.Status, []conferences.Participant, error)
}

// Store задаёт контракт зависимого компонента Store в присутствии участников и доставке realtime-событий; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - Register: операция Register с контрактом, описанным у метода.
//   - Unregister: операция Unregister с контрактом, описанным у метода.
//   - Touch: операция Touch с контрактом, описанным у метода.
//   - Get: операция получение с контрактом, описанным у метода.
//   - Active: операция Active с контрактом, описанным у метода.
//   - Prune: операция Prune с контрактом, описанным у метода.
//   - Publish: операция публикация с контрактом, описанным у метода.
//   - Subscribe: операция подписка с контрактом, описанным у метода.
type Store interface {
	// Register регистрирует физическое соединение и его ограниченное по времени присутствие.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (domain.Session): историческая физическая сессия или состояние текущего соединения.
	//   - аргумент 3 (time.Duration): срок жизни сохраняемого значения или выданного разрешения.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Register(context.Context, domain.Session, time.Duration) error
	// Unregister закрывает физическую сессию и обновляет распределённое присутствие участника.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Unregister(context.Context, string) error
	// Touch продлевает присутствие от момента подтверждённого pong, а не от
	// момента обработки запроса хранилищем.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (time.Time): время получения подтверждённого pong транспортом.
	//   - аргумент 4 (time.Duration): срок присутствия после подтверждённого pong.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Touch(context.Context, string, time.Time, time.Duration) error
	// Get читает состояние физических сессий и событий комнаты для дальнейшей обработки или ответа.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//
	// @return:
	//   - результат 1 (domain.Session): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Get(context.Context, string) (domain.Session, error)
	// Missing returns connections whose route has expired. An error must not be
	// interpreted as absence: reconciliation then leaves SQL sessions intact.
	Missing(context.Context, []string) ([]string, error)
	// Active возвращает действующие сессии, учитывая срок их активности.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//
	// @return:
	//   - результат 1 ([]domain.Session): собранные элементы результата; состав ограничивается параметрами операции.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Active(context.Context, string) ([]domain.Session, error)
	// Prune удаляет просроченные сессии и возвращает сведения для восстановления присутствия.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Prune(context.Context) error
	// Publish передаёт сохранённое изменение через транспорт событий или внутренних команд.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (domain.Bus): транспорт публикации и подписки на доверенные события.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Publish(context.Context, domain.Bus) error
	// Subscribe открывает ограниченную по времени подписку на изолированный канал событий.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//
	// @return:
	//   - результат 1 (domain.Subscription): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Subscribe(context.Context) (domain.Subscription, error)
}

// Socket задаёт контракт зависимого компонента Socket в присутствии участников и доставке realtime-событий; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - Offer: операция SDP-предложение с контрактом, описанным у метода.
//   - Stop: операция остановка с контрактом, описанным у метода.
type Socket interface {
	// Offer обрабатывает или передаёт SDP-предложение действующего WebRTC-подключения.
	//
	// @args
	//   - аргумент 1 (domain.Envelope): конверт входящего или публикуемого события.
	//
	// @return:
	//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
	Offer(domain.Envelope) bool
	// Stop останавливает активную обработку физических сессий и событий комнаты и освобождает связанные ресурсы.
	//
	// @args
	//   - аргумент 1 (string): причина завершения, отказа или изменения состояния.
	Stop(string)
}

// DisconnectObserver задаёт контракт зависимого компонента DisconnectObserver в присутствии участников и доставке realtime-событий; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - Disconnected: операция отключённый с контрактом, описанным у метода.
type DisconnectObserver interface {
	// Disconnected обрабатывает закрытие физического соединения и запускает связанное освобождение ресурсов.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (domain.Session): историческая физическая сессия или состояние текущего соединения.
	Disconnected(context.Context, domain.Session)
}

// DisconnectObservers объединяет ограниченные обработчики очистки, сохраняя независимость
// жизненных циклов отдельных подсистем.
type DisconnectObservers []DisconnectObserver

// Disconnected обрабатывает закрытие физического соединения и запускает связанное освобождение ресурсов.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - session (domain.Session): историческая физическая сессия или состояние текущего соединения.
func (observers DisconnectObservers) Disconnected(ctx context.Context, session domain.Session) {
	for _, observer := range observers {
		if observer != nil {
			observer.Disconnected(ctx, session)
		}
	}
}

// localSocket связывает локальный сокет с его сессией и управлением временем жизни.
//   - session: историческая физическая сессия или состояние текущего соединения.
//   - socket: значение socket типа Socket, используемое согласно назначению этой операции.
//   - ready: логический признак ready, управляющий соответствующей веткой обработки.
type localSocket struct {
	session domain.Session
	socket  Socket
	ready   bool
}

// Hub координирует локальные сокеты, распределённое присутствие и события с фильтрацией доступа и отдельными приоритетами.
// @params
//   - repo: хранилище постоянных данных прикладного сценария.
//   - store: значение store типа Store, используемое согласно назначению этой операции.
//   - ttl: срок жизни сохраняемого значения или выданного разрешения.
//   - logger: значение logger типа *slog.Logger, используемое согласно назначению этой операции.
//   - ctx: контекст отмены, дедлайна и времени жизни операции.
//   - cancel: отмена контекста, завершающая принадлежащие ресурсу операции.
//   - sub: значение sub типа domain.Subscription, используемое согласно назначению этой операции.
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - closing: логический признак closing, управляющий соответствующей веткой обработки.
//   - local: индекс значений local для поиска и согласования состояния.
//   - sockets: значение sockets типа sync.WaitGroup, используемое согласно назначению этой операции.
//   - workers: значение workers типа sync.WaitGroup, используемое согласно назначению этой операции.
//   - disconnectObserver: значение disconnectObserver типа DisconnectObserver, используемое согласно назначению этой операции.
//   - lowEvents: канал «low события» для передачи данных или завершения ожидания.
type Hub struct {
	repo               Repository
	store              Store
	ttl                time.Duration
	logger             *slog.Logger
	ctx                context.Context
	cancel             context.CancelFunc
	sub                domain.Subscription
	mu                 sync.Mutex
	closing            bool
	unavailable        bool
	generation         uint64
	local              map[string]*localSocket
	sockets            sync.WaitGroup
	workers            sync.WaitGroup
	disconnectObserver DisconnectObserver
	lowEvents          chan domain.Bus
	shutdownOnce       sync.Once // завершение запускается один раз при повторных вызовах
	shutdownDone       chan struct{}
}

// NewHub создаёт и связывает зависимости компонента Hub, используемого в присутствии участников и доставке realtime-событий.
//
// @args
//   - repo (Repository): хранилище постоянных данных прикладного сценария.
//   - store (Store): значение store типа Store, используемое согласно назначению этой операции.
//   - ttl (time.Duration): срок жизни сохраняемого значения или выданного разрешения.
//   - logger (*slog.Logger): значение logger типа *slog.Logger, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (*Hub): созданный компонент с переданными зависимостями.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func NewHub(repo Repository, store Store, ttl time.Duration, logger *slog.Logger) (*Hub, error) {
	ctx, cancel := context.WithCancel(context.Background())
	if logger == nil {
		logger = slog.Default()
	}
	h := &Hub{repo: repo, store: store, ttl: ttl, logger: logger, ctx: ctx, cancel: cancel, local: map[string]*localSocket{}, lowEvents: make(chan domain.Bus, 128)}
	op, c := context.WithTimeout(ctx, 5*time.Second)
	defer c()
	sub, err := store.Subscribe(op)
	if err != nil {
		cancel()
		return nil, err
	}
	h.sub = sub
	h.generation = 1
	h.workers.Add(3)
	go h.receive()
	go h.receiveLowPriority()
	go h.janitor()
	return h, nil
}

// Authorize проверяет право пользователя участвовать в операции до работы с защищёнными ресурсами.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//
// @return:
//   - результат 1 (conferences.Participant): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (h *Hub) Authorize(ctx context.Context, conferenceID, userID string) (conferences.Participant, error) {
	return h.repo.Authorize(ctx, conferenceID, userID)
}

// Check includes the live subscription in API readiness; a healthy Redis Ping
// alone does not establish that this Hub can receive conference events.
func (h *Hub) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closing || h.unavailable {
		return apperrors.ErrUnavailable
	}
	return nil
}

// SetDisconnectObserver подключает обработчик завершения физической сессии до запуска обслуживания сокетов.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - observer (DisconnectObserver): получатель сохранённых изменений конференции или закрытия сессии.
func (h *Hub) SetDisconnectObserver(observer DisconnectObserver) {
	h.mu.Lock()
	h.disconnectObserver = observer
	h.mu.Unlock()
}

// ValidateSession связывает медиа-команду с действующей разрешённой физической WebSocket-сессией.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - session (domain.Session): историческая физическая сессия или состояние текущего соединения.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (h *Hub) ValidateSession(ctx context.Context, session domain.Session) error {
	p, err := h.repo.Authorize(ctx, session.ConferenceID, session.UserID)
	if err != nil {
		return err
	}
	live, err := h.store.Get(ctx, session.ConnectionID)
	if err != nil {
		return err
	}
	if p.ID != session.ParticipantID || live.ID != session.ID || live.ConferenceID != session.ConferenceID || live.ParticipantID != session.ParticipantID || live.UserID != session.UserID || live.ConnectionID != session.ConnectionID || live.Status != "connected" {
		return apperrors.ErrForbidden
	}
	return nil
}

// Prepare подготавливает состояние WebRTC-приёма конкретной записи до обмена SDP.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//
// @return:
//   - результат 1 (domain.Session): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (h *Hub) Prepare(ctx context.Context, conferenceID, userID string) (domain.Session, error) {
	h.mu.Lock()
	closing := h.closing
	unavailable := h.unavailable
	h.mu.Unlock()
	if closing {
		return domain.Session{}, apperrors.ErrConflict
	}
	if unavailable {
		return domain.Session{}, apperrors.ErrUnavailable
	}
	p, err := h.repo.Authorize(ctx, conferenceID, userID)
	if err != nil {
		return domain.Session{}, err
	}
	now := time.Now().UTC()
	s := domain.Session{ID: uuid.NewString(), ConferenceID: conferenceID, ParticipantID: p.ID, UserID: userID, ConnectionID: uuid.NewString(), Status: "connected", ConnectedAt: now, LastSeenAt: now}
	return s, h.repo.Open(ctx, s)
}

// Abort закрывает неудачно зарегистрированную физическую сессию и освобождает её присутствие.
//
// @args
//   - session (domain.Session): историческая физическая сессия или состояние текущего соединения.
func (h *Hub) Abort(session domain.Session) {
	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	_ = h.repo.Close(ctx, session.ConnectionID, session.LastSeenAt)
}

// Register регистрирует физическое соединение и его ограниченное по времени присутствие.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - session (domain.Session): историческая физическая сессия или состояние текущего соединения.
//   - socket (Socket): значение socket типа Socket, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (h *Hub) Register(ctx context.Context, session domain.Session, socket Socket) error {
	h.mu.Lock()
	if h.closing || h.unavailable {
		err := apperrors.ErrConflict
		if h.unavailable && !h.closing {
			err = apperrors.ErrUnavailable
		}
		h.mu.Unlock()
		h.Abort(session)
		return err
	}
	h.sockets.Add(1)
	h.local[session.ConnectionID] = &localSocket{session: session, socket: socket}
	h.mu.Unlock()
	if err := h.store.Register(ctx, session, h.ttl); err != nil {
		h.Unregister(session)
		return err
	}
	// Учитываем завершение или выход одновременно с upgrade соединения и регистрацией в Redis.
	if _, err := h.repo.Authorize(ctx, session.ConferenceID, session.UserID); err != nil {
		h.Unregister(session)
		return err
	}
	state, err := h.state(ctx, session)
	if err == nil && !stateAllows(state, session.ParticipantID) {
		err = apperrors.ErrForbidden
	}
	if err != nil || !socket.Offer(domain.Event("conference.state", session.ConferenceID, stateFor(state, session))) {
		h.Unregister(session)
		if err != nil {
			return err
		}
		return apperrors.ErrConflict
	}
	h.mu.Lock()
	if entry := h.local[session.ConnectionID]; entry != nil {
		entry.ready = true
	}
	h.mu.Unlock()
	h.log(session, "connected")
	return nil
}

// Unregister закрывает физическую сессию и обновляет распределённое присутствие участника.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - session (domain.Session): историческая физическая сессия или состояние текущего соединения.
func (h *Hub) Unregister(session domain.Session) {
	h.mu.Lock()
	_, exists := h.local[session.ConnectionID]
	delete(h.local, session.ConnectionID)
	observer := h.disconnectObserver
	h.mu.Unlock()
	if !exists {
		return
	}
	defer h.sockets.Done()
	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	seen := session.LastSeenAt
	if live, err := h.store.Get(ctx, session.ConnectionID); err == nil {
		seen = live.LastSeenAt
	}
	if err := h.store.Unregister(ctx, session.ConnectionID); err != nil {
		h.log(session, "redis_cleanup_failed")
	}
	if err := h.repo.Close(ctx, session.ConnectionID, seen); err != nil {
		h.log(session, "history_cleanup_failed")
	}
	// Обработчики видят уже закрытую сессию. Две одновременно закрывающиеся вкладки
	// не могут обе принять другую вкладку за последнюю активную сессию.
	if observer != nil {
		observer.Disconnected(ctx, session)
	}
	h.log(session, "disconnected")
}

// Touch продлевает срок активности зарегистрированной физической сессии.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - session (domain.Session): историческая физическая сессия или состояние текущего соединения.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (h *Hub) Touch(ctx context.Context, session domain.Session) error {
	if _, err := h.repo.Authorize(ctx, session.ConferenceID, session.UserID); err != nil {
		return err
	}
	// Проверка членства не должна добавлять своё время к пятисекундному окну.
	// Источник LastSeenAt — только принятый транспортом подтверждённый pong.
	remaining := h.ttl - time.Since(session.LastSeenAt)
	if session.LastSeenAt.IsZero() || remaining <= 0 || remaining > h.ttl {
		return apperrors.ErrNotFound
	}
	return h.store.Touch(ctx, session.ConnectionID, session.LastSeenAt, h.ttl)
}

// GetActiveSessions возвращает действующие физические сессии для проверки медиа-команд.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//
// @return:
//   - результат 1 ([]domain.Session): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (h *Hub) GetActiveSessions(ctx context.Context, conferenceID string) ([]domain.Session, error) {
	return h.store.Active(ctx, conferenceID)
}

// Broadcast публикует доверенное событие для разрешённых получателей конференции.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - event (domain.Envelope): конверт входящего или публикуемого события.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (h *Hub) Broadcast(ctx context.Context, event domain.Envelope) error {
	return h.store.Publish(ctx, domain.Bus{Kind: "event", ConferenceID: event.ConferenceID, Event: &event})
}

// SendToParticipant публикует адресное событие физическим сессиям указанного участника.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - participantID (string): идентификатор членства участника внутри конференции.
//   - event (domain.Envelope): конверт входящего или публикуемого события.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (h *Hub) SendToParticipant(ctx context.Context, conferenceID, participantID string, event domain.Envelope) error {
	return h.store.Publish(ctx, domain.Bus{Kind: "event", ConferenceID: conferenceID, ParticipantID: participantID, Event: &event})
}

// SendToConnection публикует доверенное адресное событие конкретному физическому соединению.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - session (domain.Session): историческая физическая сессия или состояние текущего соединения.
//   - targetID (string): идентификатор связанного ресурса, заданного параметром targetID.
//   - event (domain.Envelope): конверт входящего или публикуемого события.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (h *Hub) SendToConnection(ctx context.Context, session domain.Session, targetID string, event domain.Envelope) error {
	if _, err := h.repo.Authorize(ctx, session.ConferenceID, session.UserID); err != nil {
		return err
	}
	sender, err := h.store.Get(ctx, session.ConnectionID)
	if err != nil || sender.ConferenceID != session.ConferenceID || sender.UserID != session.UserID {
		return apperrors.ErrForbidden
	}
	target, err := h.store.Get(ctx, targetID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return apperrors.New(apperrors.ErrNotFound, "target connection is offline")
	}
	if err != nil {
		return err
	}
	if target.ConferenceID != session.ConferenceID {
		return apperrors.ErrForbidden
	}
	if _, err := h.repo.Authorize(ctx, target.ConferenceID, target.UserID); err != nil {
		return apperrors.ErrForbidden
	}
	err = h.store.Publish(ctx, domain.Bus{Kind: "event", ConferenceID: session.ConferenceID, ConnectionID: targetID, Event: &event})
	if err == nil {
		h.log(session, event.Type)
	}
	return err
}

// ConferenceChanged уведомляет подключённые сессии о сохранённом изменении состояния конференции.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
func (h *Hub) ConferenceChanged(ctx context.Context, conferenceID string) {
	ctx, c := context.WithTimeout(ctx, 3*time.Second)
	defer c()
	if err := h.store.Publish(ctx, domain.Bus{Kind: "changed", ConferenceID: conferenceID}); err != nil {
		h.logger.Warn("realtime event", "conference_id", conferenceID, "event_type", "mutation_publish_failed")
	}
}

// state aggregates physical connections by persisted participant membership.
func (h *Hub) state(ctx context.Context, session domain.Session) (domain.State, error) {
	status, roster, err := h.repo.Roster(ctx, session.ConferenceID)
	if err != nil {
		return domain.State{}, err
	}
	activeSessions, err := h.store.Active(ctx, session.ConferenceID)
	if err != nil {
		return domain.State{}, err
	}
	connectionsByParticipant := map[string][]string{}
	for _, activeSession := range activeSessions {
		connectionsByParticipant[activeSession.ParticipantID] = append(connectionsByParticipant[activeSession.ParticipantID], activeSession.ConnectionID)
	}
	state := domain.State{ConnectionID: session.ConnectionID, ParticipantID: session.ParticipantID, Status: status, Participants: make([]domain.Presence, 0, len(roster))}
	for _, participant := range roster {
		connectionIDs := connectionsByParticipant[participant.ID]
		if !participant.CanParticipate() || connectionIDs == nil {
			connectionIDs = []string{}
		}
		sort.Strings(connectionIDs)
		state.Participants = append(state.Participants, domain.Presence{ParticipantView: participant.View(), Online: len(connectionIDs) > 0, Connections: len(connectionIDs), ConnectionIDs: connectionIDs})
	}
	return state, nil
}

// stateFor hides the waiting roster from non-moderators and sets the recipient identity.
func stateFor(state domain.State, session domain.Session) domain.State {
	state.ConnectionID, state.ParticipantID = session.ConnectionID, session.ParticipantID
	isModerator := false
	for _, participant := range state.Participants {
		if participant.ID == session.ParticipantID {
			isModerator = participant.Role == conferences.Owner || participant.Role == conferences.CoHost
			break
		}
	}
	if isModerator {
		return state
	}
	visible := make([]domain.Presence, 0, len(state.Participants))
	for _, participant := range state.Participants {
		if participant.CanReadHistory() {
			visible = append(visible, participant)
		}
	}
	state.Participants = visible
	return state
}

// stateAllows проверяет, допускает ли актуальный снимок подключение данной сессии к комнате.
//
// @args
//   - state (domain.State): значение state типа domain.State, используемое согласно назначению этой операции.
//   - participantID (string): идентификатор членства участника внутри конференции.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func stateAllows(state domain.State, participantID string) bool {
	if state.Status != conferences.Created && state.Status != conferences.Active {
		return false
	}
	for _, p := range state.Participants {
		if p.ID == participantID {
			return p.CanParticipate() && !p.ChatLeft
		}
	}
	return false
}

// receive читает входящие события или пакеты и передаёт их соответствующим обработчикам.
func (h *Hub) receive() {
	defer h.workers.Done()
	sub, generation := h.sub, h.generation // Set before this worker starts.
	for {
		bus, err := sub.Receive(h.ctx)
		if err != nil {
			if h.ctx.Err() != nil {
				return
			}
			h.logger.Warn("realtime event", "event_type", "broker_unavailable")
			h.breakBroker(generation)
			_ = sub.Close()
			var ok bool
			sub, generation, ok = h.recoverSubscription()
			if !ok {
				return
			}
			continue
		}
		if bus.Kind == "event" && bus.Event != nil && domain.LowPriorityEvent(bus.Event.Type) {
			select {
			case h.lowEvents <- bus:
			default:
			} // Сохранённая история восстанавливает события после переполнения.
		} else {
			h.deliver(bus)
		}
	}
}

// breakBroker invalidates only the generation whose operation failed. A slow
// Prune result from an old connection must not close its healthy replacement.
// Network close and socket notifications run outside the registry lock.
func (h *Hub) breakBroker(generation uint64) {
	h.mu.Lock()
	if h.closing || h.unavailable || h.generation != generation {
		h.mu.Unlock()
		return
	}
	h.unavailable = true
	sub := h.sub
	entries := make([]*localSocket, 0, len(h.local))
	for _, entry := range h.local {
		entries = append(entries, entry)
	}
	h.mu.Unlock()
	for _, entry := range entries {
		entry.socket.Stop("broker_unavailable")
	}
	_ = sub.Close() // Interrupt Receive so its sole owner starts recovery.
}

// recoverSubscription runs in the existing receive worker. Each attempt owns
// its timeout and the next subscription is acknowledged before admission opens.
func (h *Hub) recoverSubscription() (domain.Subscription, uint64, bool) {
	backoff := 100 * time.Millisecond
	for {
		timer := time.NewTimer(backoff)
		select {
		case <-h.ctx.Done():
			timer.Stop()
			return nil, 0, false
		case <-timer.C:
		}
		timer.Stop()
		ctx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
		sub, err := h.store.Subscribe(ctx)
		cancel()
		if err != nil {
			backoff = min(backoff*2, 2*time.Second)
			continue
		}
		h.mu.Lock()
		if h.closing || h.ctx.Err() != nil {
			h.mu.Unlock()
			_ = sub.Close()
			return nil, 0, false
		}
		h.sub = sub
		h.generation++
		generation := h.generation
		h.unavailable = false
		h.mu.Unlock()
		h.logger.Info("realtime event", "event_type", "broker_recovered")
		return sub, generation, true
	}
}

// receiveLowPriority доставляет события чата и реакций через отдельную ограниченную очередь.
func (h *Hub) receiveLowPriority() {
	defer h.workers.Done()
	for {
		select {
		case <-h.ctx.Done():
			return
		case bus := <-h.lowEvents:
			h.deliver(bus)
		}
	}
}

// entries снимает список локальных сокетов под блокировкой для дальнейшей работы вне неё.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//
// @return:
//   - результат 1 ([]*localSocket): собранные элементы результата; состав ограничивается параметрами операции.
func (h *Hub) entries(conferenceID string) []*localSocket {
	h.mu.Lock()
	defer h.mu.Unlock()
	rows := make([]*localSocket, 0, len(h.local))
	for _, entry := range h.local {
		if entry.ready && (conferenceID == "" || entry.session.ConferenceID == conferenceID) {
			rows = append(rows, entry)
		}
	}
	return rows
}

// deliver проверяет получателей события и ставит его в соответствующие локальные очереди сокетов.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - bus (domain.Bus): транспорт публикации и подписки на доверенные события.
func (h *Hub) deliver(bus domain.Bus) {
	ctx, c := context.WithTimeout(h.ctx, 5*time.Second)
	defer c()
	if bus.Kind == "expired" && bus.Session != nil {
		_ = h.repo.Close(ctx, bus.Session.ConnectionID, bus.Session.LastSeenAt)
		h.mu.Lock()
		observer := h.disconnectObserver
		h.mu.Unlock()
		if observer != nil {
			observer.Disconnected(ctx, *bus.Session)
		}
	}
	entries := h.entries(bus.ConferenceID)
	if len(entries) == 0 {
		return
	}
	if bus.Kind == "event" && bus.Event != nil {
		// Заново проверяем получателей по текущему сохранённому членству, а не кешу сокета.
		// Удаление участника или завершение может зафиксироваться до доставки события изменения.
		kind := bus.Event.Type
		restricted := strings.HasPrefix(kind, "caption.") || strings.HasPrefix(kind, "chat.") || strings.HasPrefix(kind, "recording.") || strings.HasPrefix(kind, "reaction.") || strings.HasPrefix(kind, "participant.")
		allowed := map[string]bool{}
		if restricted {
			status, roster, err := h.repo.Roster(ctx, bus.ConferenceID)
			if err != nil {
				return
			}
			for _, p := range roster {
				ok := p.CanParticipate() && !p.ChatLeft
				if status == conferences.Finished || status == conferences.Cancelled {
					ok = false
				}
				if kind == "participant.waiting" || kind == "participant.rejected" {
					ok = ok && p.CanAdmit()
				}
				allowed[p.ID] = ok
			}
		}
		for _, entry := range entries {
			s := entry.session
			if restricted && !allowed[s.ParticipantID] {
				continue
			}
			if (bus.ConnectionID == "" || bus.ConnectionID == s.ConnectionID) && (bus.ParticipantID == "" || bus.ParticipantID == s.ParticipantID) {
				if !entry.socket.Offer(*bus.Event) {
					entry.socket.Stop("slow_client")
				}
			}
		}
		return
	}
	if len(entries) == 0 {
		return
	}
	// Один снимок на событие конференции вместо отдельного обращения к БД и Redis для каждого сокета.
	state, err := h.state(ctx, entries[0].session)
	if err != nil {
		for _, e := range entries {
			e.socket.Stop("state_unavailable")
		}
		return
	}
	joined := map[string]bool{}
	for _, p := range state.Participants {
		joined[p.ID] = p.CanParticipate() && !p.ChatLeft
	}
	for _, entry := range entries {
		if state.Status == conferences.Finished || state.Status == conferences.Cancelled || !joined[entry.session.ParticipantID] {
			entry.socket.Stop("membership_closed")
			continue
		}
		kind := "conference.state"
		if bus.Kind == "connected" {
			kind = "participant.connected"
		} else if bus.Kind == "disconnected" || bus.Kind == "expired" {
			kind = "participant.disconnected"
		}
		if !entry.socket.Offer(domain.Event(kind, bus.ConferenceID, stateFor(state, entry.session))) {
			entry.socket.Stop("slow_client")
		}
	}
}

// janitor периодически очищает истёкшие присутствия и закрывает утратившие авторизацию сокеты.
func (h *Hub) janitor() {
	defer h.workers.Done()
	interval := h.ttl / 4
	if interval > 250*time.Millisecond {
		interval = 250 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	cursor := ""
	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			ctx, c := context.WithTimeout(h.ctx, 5*time.Second)
			h.mu.Lock()
			generation := h.generation
			h.mu.Unlock()
			if err := h.store.Prune(ctx); err != nil {
				h.breakBroker(generation)
			}
			// Восстанавливаем и сбой между открытием сессии в SQL и регистрацией в Redis.
			rows, err := h.repo.Stale(ctx, time.Now().Add(-2*h.ttl), cursor)
			if err == nil {
				h.closeMissingSessions(ctx, rows)
				if len(rows) < 500 {
					cursor = ""
				} else {
					cursor = rows[len(rows)-1].ID
				}
			}
			c()
		}
	}
}

// closeMissingSessions checks one bounded SQL page in a Redis request batch.
// Live routes need no JSON decoding; SQL is changed only after a successful
// presence check, preserving the failure behavior of the former GET loop.
func (h *Hub) closeMissingSessions(ctx context.Context, rows []domain.Session) {
	if len(rows) == 0 {
		return
	}
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].ConnectionID
	}
	missing, err := h.store.Missing(ctx, ids)
	if err != nil || len(missing) == 0 {
		return
	}
	absent := make(map[string]struct{}, len(missing))
	for _, id := range missing {
		absent[id] = struct{}{}
	}
	for _, s := range rows {
		if _, ok := absent[s.ConnectionID]; ok {
			_ = h.repo.Close(ctx, s.ConnectionID, s.LastSeenAt)
		}
	}
}

// stopSockets закрывает выбранные локальные сокеты после отзыва доступа или смены состояния конференции.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - reason (string): причина завершения, отказа или изменения состояния.
func (h *Hub) stopSockets(reason string) {
	h.mu.Lock()
	h.closing = true
	entries := make([]*localSocket, 0, len(h.local))
	for _, e := range h.local {
		entries = append(entries, e)
	}
	h.mu.Unlock()
	for _, e := range entries {
		e.socket.Stop(reason)
	}
}

// Shutdown останавливает менеджер и ожидает завершения принадлежащих ему ресурсов.
func (h *Hub) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = h.ShutdownContext(ctx)
}

// ShutdownContext запрещает новые сокеты и ожидает завершения сокетов, PubSub и
// фоновых задач. ctx ограничивает ожидание при плавном развёртывании; при его
// отмене принудительно отменяется контекст Hub и возвращается ctx.Err(). Повторные
// вызовы ждут ту же остановку, не запускают дополнительные горутины очистки.
func (h *Hub) ShutdownContext(ctx context.Context) error {
	h.shutdownOnce.Do(func() {
		h.shutdownDone = make(chan struct{})
		go func() {
			defer close(h.shutdownDone)
			h.stopSockets("server_shutdown")
			h.cancel()
			h.mu.Lock()
			sub := h.sub
			h.mu.Unlock()
			_ = sub.Close()
			h.sockets.Wait()
			h.workers.Wait()
		}()
	})
	select {
	case <-h.shutdownDone:
		return nil
	case <-ctx.Done():
		h.cancel()
		return ctx.Err()
	}
}

// LocalCount возвращает число зарегистрированных локальных сокетов под блокировкой Hub.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @return:
//   - результат 1 (int): значение, подготовленное операцией для вызывающей стороны.
func (h *Hub) LocalCount() int { h.mu.Lock(); defer h.mu.Unlock(); return len(h.local) }

// log записывает ограниченную диагностику компонента с указанными параметрами.
//
// @args
//   - s (domain.Session): значение s типа domain.Session, используемое согласно назначению этой операции.
//   - kind (string): тип события, ошибки или медиа, определяющий ветку обработки.
func (h *Hub) log(s domain.Session, kind string) {
	h.logger.Info("realtime event", "conference_id", s.ConferenceID, "participant_id", s.ParticipantID, "user_id", s.UserID, "connection_id", s.ConnectionID, "event_type", kind)
}

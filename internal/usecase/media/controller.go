package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/config"
	domain "github.com/janickiy/meet-space/internal/domain/media"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

// Registry задаёт контракт зависимого компонента Registry в защищённом управлении медиа-комнатой; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - Workers: операция Workers с контрактом, описанным у метода.
//   - Claim: операция Claim с контрактом, описанным у метода.
//   - GetOwner: операция получение владелец с контрактом, описанным у метода.
type Registry interface {
	// Workers возвращает действующие медиа-воркеры для распределения комнаты.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//
	// @return:
	//   - результат 1 ([]domain.Worker): собранные элементы результата; состав ограничивается параметрами операции.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Workers(context.Context) ([]domain.Worker, error)
	// Claim пытается закрепить распределённое владение ресурсом за указанным воркером.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 3 (string): идентификатор воркера-владельца операции.
	//   - аргумент 4 (time.Duration): срок жизни сохраняемого значения или выданного разрешения.
	//
	// @return:
	//   - результат 1 (domain.Route): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Claim(context.Context, string, string, time.Duration) (domain.Route, error)
	// GetOwner читает актуального владельца медиа-комнаты и его версию владения.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//
	// @return:
	//   - результат 1 (domain.Route): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	GetOwner(context.Context, string) (domain.Route, error)
}

// Tickets задаёт контракт зависимого компонента Tickets в защищённом управлении медиа-комнатой; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - Issue: операция Issue с контрактом, описанным у метода.
type Tickets interface {
	// Issue выпускает подписанный JWT пользователя с настроенным сроком действия.
	//
	// @args
	//   - аргумент 1 (domain.Binding): проверенная идентичность медиа-подключения, назначенная сервером.
	//   - аргумент 2 (domain.Route): адрес и версия действующего владельца медиа-комнаты.
	//
	// @return:
	//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Issue(domain.Binding, domain.Route) (string, error)
}

// Transport задаёт контракт зависимого компонента Transport в защищённом управлении медиа-комнатой; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - Call: операция Call с контрактом, описанным у метода.
type Transport interface {
	// Call выполняет защищённый внутренний HTTP-вызов выбранной операции медиа-воркера.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): действие управления, которое необходимо проверить или исполнить.
	//   - аргумент 3 (domain.Command): внутренняя команда с типом операции и серверной идентичностью ресурса.
	//
	// @return:
	//   - результат 1 (domain.Result): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Call(context.Context, string, domain.Command) (domain.Result, error)
}

// Sessions задаёт контракт зависимого компонента Sessions в защищённом управлении медиа-комнатой; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - ValidateSession: операция Validate сессия с контрактом, описанным у метода.
type Sessions interface {
	// ValidateSession связывает медиа-команду с действующей разрешённой физической WebSocket-сессией.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (realtime.Session): историческая физическая сессия или состояние текущего соединения.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	ValidateSession(context.Context, realtime.Session) error
}

// Publisher задаёт контракт зависимого компонента Publisher в защищённом управлении медиа-комнатой; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - Publish: операция публикация с контрактом, описанным у метода.
type Publisher interface {
	// Publish передаёт сохранённое изменение через транспорт событий или внутренних команд.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (realtime.Bus): транспорт публикации и подписки на доверенные события.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Publish(context.Context, realtime.Bus) error
}

// PolicyProvider задаёт контракт зависимого компонента PolicyProvider в защищённом управлении медиа-комнатой; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - MediaPolicy: операция медиа политика с контрактом, описанным у метода.
type PolicyProvider interface {
	// MediaPolicy читает действующие серверные ограничения передачи медиа участника.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 3 (string): идентификатор членства участника внутри конференции.
	//
	// @return:
	//   - результат 1 (domain.ParticipantPolicy): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	MediaPolicy(context.Context, string, string) (domain.ParticipantPolicy, error)
}

// endpoint задаёт согласованное представление данных «адрес сервиса» для защищённом управлении медиа-комнатой.
// @params
//   - binding: проверенная идентичность медиа-подключения, назначенная сервером.
//   - route: адрес и версия действующего владельца медиа-комнаты.
//   - peerID: идентификатор связанного ресурса, заданного параметром peerID.
type endpoint struct {
	binding domain.Binding
	route   domain.Route
	peerID  string
}

// Controller связывает клиентскую сигнализацию с авторизацией сессии, владельцем комнаты и внутренним медиа-транспортом.
// @params
//   - registry: распределённый реестр воркеров и владения комнатами.
//   - tickets: сервис выпуска и проверки ограниченных билетов подключения.
//   - transport: клиент защищённого внутреннего медиа-транспорта.
//   - sessions: хранилище и авторизация физических сессий подключения.
//   - publisher: транспорт публикации событий после сохранения состояния.
//   - cfg: проверенные настройки соответствующего компонента.
//   - limits: настройки ограничений размера, частоты и количества ресурсов.
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - peers: индекс значений peers для поиска и согласования состояния.
//   - policyProvider: значение policyProvider типа PolicyProvider, используемое согласно назначению этой операции.
type Controller struct {
	registry       Registry
	tickets        Tickets
	transport      Transport
	sessions       Sessions
	publisher      Publisher
	cfg            config.MediaConfig
	limits         config.RealtimeConfig
	mu             sync.Mutex
	peers          map[string]endpoint
	policyProvider PolicyProvider
}

// SetPolicyProvider подключает источник сохранённых полномочий участников до начала обработки запросов.
//
// @args
//   - provider (PolicyProvider): источник актуальной сохранённой политики участника.
func (c *Controller) SetPolicyProvider(provider PolicyProvider) { c.policyProvider = provider }

// policy получает серверные ограничения медиа участника из сохранённой модели.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - binding (domain.Binding): проверенная идентичность медиа-подключения, назначенная сервером.
//
// @return:
//   - результат 1 (*domain.ParticipantPolicy): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Controller) policy(ctx context.Context, binding domain.Binding) (*domain.ParticipantPolicy, error) {
	if c.policyProvider == nil {
		return nil, nil
	}
	policy, err := c.policyProvider.MediaPolicy(ctx, binding.ConferenceID, binding.ParticipantID)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}
	return &policy, nil
}

// SetParticipantPolicy передаёт актуальную политику участника владельцу медиа-комнаты.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - participantID (string): идентификатор членства участника внутри конференции.
//   - policy (domain.ParticipantPolicy): актуальные ограничения медиа и версия модерации участника.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Controller) SetParticipantPolicy(ctx context.Context, conferenceID, participantID string, policy domain.ParticipantPolicy) error {
	if !validUUID(conferenceID) || !validUUID(participantID) || policy.Version < 0 {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.OperationTimeout)
	defer cancel()
	route, err := c.registry.GetOwner(ctx, conferenceID)
	if errors.Is(err, domain.ErrOwnership) {
		return nil
	}
	if err != nil {
		return domain.ErrUnavailable
	}
	_, err = c.transport.Call(ctx, "policy", domain.Command{RequestID: uuid.NewString(), ConferenceID: conferenceID, ParticipantID: participantID, Route: route, Policy: &policy})
	return err
}

// CloseConference закрывает все медиа-подключения и ресурсы конкретной конференции.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Controller) CloseConference(ctx context.Context, conferenceID string) error {
	if !validUUID(conferenceID) {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.OperationTimeout)
	defer cancel()
	route, err := c.registry.GetOwner(ctx, conferenceID)
	if errors.Is(err, domain.ErrOwnership) {
		return nil
	}
	if err != nil {
		return domain.ErrUnavailable
	}
	_, err = c.transport.Call(ctx, "close", domain.Command{RequestID: uuid.NewString(), ConferenceID: conferenceID, Route: route})
	return err
}

// NewController создаёт и связывает зависимости компонента Controller, используемого в защищённом управлении медиа-комнатой.
//
// @args
//   - registry (Registry): распределённый реестр воркеров и владения комнатами.
//   - tickets (Tickets): сервис выпуска и проверки ограниченных билетов подключения.
//   - transport (Transport): клиент защищённого внутреннего медиа-транспорта.
//   - sessions (Sessions): хранилище и авторизация физических сессий подключения.
//   - publisher (Publisher): транспорт публикации событий после сохранения состояния.
//   - cfg (config.MediaConfig): проверенные настройки соответствующего компонента.
//   - limits (config.RealtimeConfig): настройки ограничений размера, частоты и количества ресурсов.
//
// @return:
//   - результат 1 (*Controller): созданный компонент с переданными зависимостями.
func NewController(registry Registry, tickets Tickets, transport Transport, sessions Sessions, publisher Publisher, cfg config.MediaConfig, limits config.RealtimeConfig) *Controller {
	limits.SDPBytes = min(limits.SDPBytes, domain.MaxSDPBytes)
	limits.ICEBytes = min(limits.ICEBytes, domain.MaxICEBytes)
	return &Controller{registry: registry, tickets: tickets, transport: transport, sessions: sessions, publisher: publisher, cfg: cfg, limits: limits, peers: map[string]endpoint{}}
}

// Handle обрабатывает проверенное событие сигнализации в рамках живой серверной сессии.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - session (realtime.Session): историческая физическая сессия или состояние текущего соединения.
//   - expiresAt (time.Time): момент окончания действия сессии, токена или аренды.
//   - event (realtime.Envelope): конверт входящего или публикуемого события.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Controller) Handle(ctx context.Context, session realtime.Session, expiresAt time.Time, event realtime.Envelope) error {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.OperationTimeout)
	defer cancel()
	if !expiresAt.After(time.Now()) || c.sessions.ValidateSession(ctx, session) != nil {
		return domain.ErrUnauthorized
	}
	signal, err := c.decode(event.Type, event.Data)
	if err != nil {
		return err
	}
	binding := domain.Binding{ConferenceID: session.ConferenceID, ParticipantID: session.ParticipantID, SessionID: session.ID, ConnectionID: session.ConnectionID, UserID: session.UserID, AuthorizationExpiresAt: expiresAt}
	if event.Type == "media.join" {
		return c.join(ctx, binding, event.ID)
	}
	c.mu.Lock()
	peer, exists := c.peers[session.ConnectionID]
	c.mu.Unlock()
	if !exists {
		if event.Type == "media.leave" {
			return c.emit(ctx, binding, "media.left", event.ID, map[string]string{"mediaPeerId": signal.MediaPeerID})
		}
		return domain.ErrPeerNotFound
	}
	// Пустая команда media.leave отменяет присоединение, поставленное в очередь до получения
	// браузером ID соединения. Единственный читатель WS выполняет её после ответа на join;
	// очищать разрешено только авторизованную сессию отправителя.
	if signal.MediaPeerID != peer.peerID && !(event.Type == "media.leave" && signal.MediaPeerID == "") {
		return domain.ErrUnauthorized
	}
	owner, err := c.registry.GetOwner(ctx, session.ConferenceID)
	if err != nil {
		return domain.ErrOwnership
	}
	if owner != peer.route {
		return domain.ErrOwnership
	}
	command := domain.Command{RequestID: event.ID, Binding: peer.binding, Route: peer.route, MediaPeerID: peer.peerID, NegotiationID: signal.NegotiationID, SDP: signal.SDP, Candidate: signal.Candidate, TrackID: signal.TrackID}
	command.Publications = signal.Publications
	if event.Type == "media.offer" {
		command.Policy, err = c.policy(ctx, binding)
		if err != nil {
			return err
		}
	}
	action := strings.TrimPrefix(event.Type, "media.")
	result, err := c.transport.Call(ctx, action, command)
	if err != nil {
		return err
	}
	if action == "offer" {
		if result.MediaPeerID != peer.peerID || result.SDP == "" || len(result.SDP) > c.limits.SDPBytes {
			return domain.ErrUnavailable
		}
		return c.emit(ctx, binding, "media.answer", event.ID, map[string]string{"mediaPeerId": peer.peerID, "negotiationId": signal.NegotiationID, "sdp": result.SDP})
	}
	if action == "leave" {
		c.mu.Lock()
		delete(c.peers, session.ConnectionID)
		c.mu.Unlock()
		return c.emit(ctx, binding, "media.left", event.ID, map[string]string{"mediaPeerId": peer.peerID})
	}
	return c.emit(ctx, binding, "ack", event.ID, map[string]string{"type": event.Type})
}

// join назначает владельца медиа-комнаты и создаёт подключение для проверенной физической сессии.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - binding (domain.Binding): проверенная идентичность медиа-подключения, назначенная сервером.
//   - requestID (string): идентификатор запроса сигнализации для сопоставления ответа.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Controller) join(ctx context.Context, binding domain.Binding, requestID string) error {
	policy, err := c.policy(ctx, binding)
	if err != nil {
		return err
	}
	if policy != nil && policy.Kicked {
		return domain.ErrPolicy
	}
	workers, err := c.registry.Workers(ctx)
	if err != nil || len(workers) == 0 {
		return domain.ErrUnavailable
	}
	// Небольшой стабильный хеш распределяет новые конференции; Claim сохраняет текущего
	// работающего владельца. Это маршрутизация, а не распределённый планировщик.
	conf, _ := uuid.Parse(binding.ConferenceID)
	index := int(conf[0]) % len(workers)
	route, err := c.registry.Claim(ctx, binding.ConferenceID, workers[index].ID, c.cfg.OwnershipTTL)
	if err != nil {
		return domain.ErrUnavailable
	}
	ticket, err := c.tickets.Issue(binding, route)
	if err != nil {
		return domain.ErrUnauthorized
	}
	result, err := c.transport.Call(ctx, "join", domain.Command{RequestID: requestID, Binding: binding, Route: route, Ticket: ticket, Policy: policy})
	if err != nil {
		return err
	}
	if parsed, err := uuid.Parse(result.MediaPeerID); err != nil || parsed == uuid.Nil || result.MaxPeers < 2 || result.MaxPeers > 32 || result.WorkerID != route.WorkerID {
		return domain.ErrUnavailable
	}
	video := result.VideoCapture
	if video.MaxWidth < 1 || video.MaxWidth > 1280 || video.MaxHeight < 1 || video.MaxHeight > 720 || video.MaxFrameRate < 1 || video.MaxFrameRate > 30 {
		// Воркер подготовил соединение, но неверный источник захвата может не дойти до браузера.
		// Освобождаем подготовленное медиа-соединение, чтобы не оставить его без владельца.
		cleanup, cancel := context.WithTimeout(context.Background(), min(c.cfg.OperationTimeout, 3*time.Second))
		_, _ = c.transport.Call(cleanup, "leave", domain.Command{RequestID: uuid.NewString(), Binding: binding, Route: route, MediaPeerID: result.MediaPeerID})
		cancel()
		return domain.ErrUnavailable
	}
	peer := endpoint{binding: binding, route: route, peerID: result.MediaPeerID}
	c.mu.Lock()
	c.peers[binding.ConnectionID] = peer
	c.mu.Unlock()
	if result.Tracks == nil {
		result.Tracks = []domain.Track{}
	}
	if result.ICEServers == nil {
		result.ICEServers = []realtime.ICEServer{}
	}
	err = c.emit(ctx, binding, "media.joined", requestID, map[string]any{"mediaPeerId": result.MediaPeerID, "workerId": result.WorkerID, "maxPeers": result.MaxPeers, "iceServers": result.ICEServers, "tracks": result.Tracks, "videoCapture": result.VideoCapture, "policy": result.Policy})
	if err != nil {
		c.Disconnected(context.Background(), realtime.Session{ConnectionID: binding.ConnectionID})
	}
	return err
}

// emit формирует и передаёт исходящее событие через принадлежащий компоненту канал доставки.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - binding (domain.Binding): проверенная идентичность медиа-подключения, назначенная сервером.
//   - kind (string): тип события, ошибки или медиа, определяющий ветку обработки.
//   - replyTo (string): идентификатор исходного запроса или сообщения, на которое даётся ответ.
//   - data (any): полезная нагрузка события или байты обрабатываемого содержимого.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Controller) emit(ctx context.Context, binding domain.Binding, kind, replyTo string, data any) error {
	event := realtime.Event(kind, binding.ConferenceID, data)
	event.ReplyTo = replyTo
	if err := c.publisher.Publish(ctx, realtime.Bus{Kind: "event", ConferenceID: binding.ConferenceID, ConnectionID: binding.ConnectionID, Event: &event}); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

// Disconnected обрабатывает закрытие физического соединения и запускает связанное освобождение ресурсов.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - session (realtime.Session): историческая физическая сессия или состояние текущего соединения.
func (c *Controller) Disconnected(ctx context.Context, session realtime.Session) {
	c.mu.Lock()
	peer, exists := c.peers[session.ConnectionID]
	delete(c.peers, session.ConnectionID)
	c.mu.Unlock()
	if !exists {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, min(c.cfg.OperationTimeout, 3*time.Second))
	defer cancel()
	_, _ = c.transport.Call(ctx, "leave", domain.Command{RequestID: uuid.NewString(), Binding: peer.binding, Route: peer.route, MediaPeerID: peer.peerID})
}

// strictDecode разбирает JSON без неизвестных полей, чтобы клиент не передавал неподдерживаемые параметры.
//
// @args
//   - raw ([]byte): исходные байты JSON, пакета или сериализованного значения.
//   - target (any): целевой объект, участник или состояние операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func strictDecode(raw []byte, target any) error {
	if !utf8.Valid(raw) {
		return domain.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		return domain.ErrInvalid
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.ErrInvalid
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

// decode строго разбирает нагрузку разрешённого вида медиа-сигнализации.
//
// @args
//   - kind (string): тип события, ошибки или медиа, определяющий ветку обработки.
//   - raw (json.RawMessage): исходные байты JSON, пакета или сериализованного значения.
//
// @return:
//   - результат 1 (domain.Signal): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Controller) decode(kind string, raw json.RawMessage) (domain.Signal, error) {
	var signal domain.Signal
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return signal, domain.ErrInvalid
	}
	if strictDecode(raw, &signal) != nil {
		return signal, domain.ErrInvalid
	}
	if kind != "media.offer" && signal.Publications != nil {
		return signal, domain.ErrInvalid
	}
	if kind == "media.join" {
		if signal.MediaPeerID != "" || signal.NegotiationID != "" || signal.SDP != "" || len(signal.Candidate) != 0 || signal.TrackID != "" {
			return signal, domain.ErrInvalid
		}
		return signal, nil
	}
	if !validUUID(signal.MediaPeerID) && !(kind == "media.leave" && signal.MediaPeerID == "") {
		return signal, domain.ErrInvalid
	}
	switch kind {
	case "media.offer":
		if !validUUID(signal.NegotiationID) || signal.SDP == "" || len(signal.SDP) > c.limits.SDPBytes || len(signal.Candidate) != 0 || signal.TrackID != "" {
			return signal, domain.ErrInvalid
		}
		if len(signal.Publications) > 4 {
			return signal, domain.ErrLimit
		}
		mids := map[string]bool{}
		sources := map[domain.Source]bool{}
		for _, pub := range signal.Publications {
			if pub.MID == "" || len(pub.MID) > 64 || strings.ContainsAny(pub.MID, "\r\n\x00 ") || len(pub.TrackID) > 128 || strings.ContainsAny(pub.TrackID, "\r\n\x00") || domain.SourceKind(pub.Source) == "" || mids[pub.MID] || sources[pub.Source] {
				return signal, domain.ErrInvalid
			}
			mids[pub.MID] = true
			sources[pub.Source] = true
		}
	case "media.ready":
		if !validUUID(signal.NegotiationID) || signal.SDP != "" || len(signal.Candidate) != 0 || signal.TrackID != "" {
			return signal, domain.ErrInvalid
		}
	case "media.ice":
		if signal.NegotiationID != "" || signal.SDP != "" || signal.TrackID != "" || len(signal.Candidate) == 0 || len(signal.Candidate) > c.limits.ICEBytes {
			return signal, domain.ErrInvalid
		}
		if string(signal.Candidate) != "null" {
			var ice struct {
				Candidate        string  `json:"candidate"`
				SDPMid           *string `json:"sdpMid"`
				SDPMLineIndex    *uint16 `json:"sdpMLineIndex"`
				UsernameFragment *string `json:"usernameFragment"`
			}
			if strictDecode(signal.Candidate, &ice) != nil || strings.ContainsAny(ice.Candidate, "\r\n\x00") || (ice.SDPMid != nil && len(*ice.SDPMid) > 128) || (ice.UsernameFragment != nil && len(*ice.UsernameFragment) > 256) {
				return signal, domain.ErrInvalid
			}
		}
	case "media.leave":
		if signal.NegotiationID != "" || signal.SDP != "" || len(signal.Candidate) != 0 || signal.TrackID != "" {
			return signal, domain.ErrInvalid
		}
	case "media.unpublish":
		if signal.NegotiationID != "" || signal.SDP != "" || len(signal.Candidate) != 0 || len(signal.TrackID) < 1 || len(signal.TrackID) > 128 || strings.ContainsAny(signal.TrackID, "\r\n\x00") {
			return signal, domain.ErrInvalid
		}
	default:
		return signal, domain.ErrInvalid
	}
	return signal, nil
}

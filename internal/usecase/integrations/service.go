package integrations

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"net/url"
	"strconv"
	"strings"
	texttemplate "text/template"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	d "github.com/janickiy/go-recorder/internal/domain/integrations"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	n "github.com/janickiy/go-recorder/internal/domain/notifications"
)

// Options задаёт публичный frontend URL, общий timeout провайдеров и конечный набор UTC offsets напоминаний.
type Options struct {
	PublicURL          string
	ReminderOffsets    []time.Duration
	ProviderTimeout    time.Duration
	MockConnectAllowed bool
}

// ConferenceSnapshot представляет актуальные поля для безопасной внешней синхронизации, не включая media-путь.
type ConferenceSnapshot struct {
	ID, OwnerID, Title, InviteCode, Status string
	ScheduledAt                            *time.Time
	PlannedDurationMin                     *int
	IntegrationVersion                     int64
}

// Delivery собирает разрешённый факт уведомления и адрес получателя, не раскрывая transcript.
type Delivery struct {
	Notification n.Notification
	Email        string
	Preferences  d.Preferences
	Allowed      bool
}

// Repository задаёт постоянные операции интеграций; async записи должны проверять lease задания в своей транзакции.
type Repository interface {
	// Preferences читает собственные настройки с privacy defaults.
	// @args контекст — отмена; строка — userID.
	// @return: настройки либо ошибка DB.
	Preferences(context.Context, string) (d.Preferences, error)
	// SavePreferences атомарно заменяет категории/каналы владельца.
	// @args контекст — отмена; настройки — boolean поля и доверенный userID.
	// @return: ошибка DB.
	SavePreferences(context.Context, d.Preferences) error
	// Devices читает не более установленного предела собственных регистраций.
	// @args контекст — отмена; строка — userID; bool — только active устройства.
	// @return: регистрации без публичного раскрытия токенов либо ошибка.
	Devices(context.Context, string, bool) ([]d.Device, error)
	// SaveDevice идемпотентно сохраняет encrypted provider token.
	// @args контекст — отмена; устройство — owner, fingerprint и ciphertext.
	// @return: ошибка quota/DB.
	SaveDevice(context.Context, d.Device) error
	// RevokeDevice удаляет sensitive ciphertext собственного устройства.
	// @args контекст — отмена; строки — owner userID и deviceID.
	// @return: not found для чужого устройства либо ошибка DB.
	RevokeDevice(context.Context, string, string) error
	// Connections читает собственные активные/отозванные подключения.
	// @args контекст — отмена; строка — owner userID.
	// @return: ограниченный список либо ошибка.
	Connections(context.Context, string) ([]d.CalendarConnection, error)
	// SaveConnection сохраняет явно авторизованное новое подключение/reconnect.
	// @args контекст — отмена; подключение — owner и encrypted OAuth grant.
	// @return: ошибка DB.
	SaveConnection(context.Context, d.CalendarConnection) error
	// RefreshConnection меняет token rotation только действующего прежнего grant.
	// @args контекст — отмена; подключение — новые credentials; строка — прежний refresh ciphertext.
	// @return: retryable конфликт при revoke/rotation либо ошибка DB.
	RefreshConnection(context.Context, d.CalendarConnection, string) error
	// RevokeConnection отключает локальный grant и очищает tokens.
	// @args контекст — отмена; строки — owner userID и connectionID.
	// @return: not found/ошибка DB.
	RevokeConnection(context.Context, string, string) error
	// SaveOAuthState сохраняет одноразовый state с encrypted verifier и конечным TTL.
	// @args контекст — отмена; state — user/provider binding и hash.
	// @return: quota/DB ошибка.
	SaveOAuthState(context.Context, d.OAuthState) error
	// TakeOAuthState одноразово забирает неистёкший state правильного владельца.
	// @args контекст — отмена; строки — userID, provider и state hash.
	// @return: encrypted verifier либо безопасная ошибка проверки.
	TakeOAuthState(context.Context, string, string, string) (d.OAuthState, error)
	// CalendarMappings читает только разрешённую owner/cohost проекцию.
	// @args контекст — отмена; строки — userID и conferenceID.
	// @return: mappings либо ошибка доступа.
	CalendarMappings(context.Context, string, string) ([]d.CalendarMapping, error)
	// Conference читает актуальное расписание для version fencing.
	// @args контекст — отмена; строка — conferenceID.
	// @return: ограниченный snapshot либо ошибка.
	Conference(context.Context, string) (ConferenceSnapshot, error)
	// MappingsForSync читает внутренние mappings доверенного worker.
	// @args контекст — отмена; строка — conferenceID.
	// @return: ограниченный список либо ошибка.
	MappingsForSync(context.Context, string) ([]d.CalendarMapping, error)
	// SaveMapping проверяет lease, current schedule version и active connection перед commit.
	// @args контекст — отмена; job — lease/version; mapping — provider result.
	// @return: fenced DB ошибка либо nil.
	SaveMapping(context.Context, jobs.Job, d.CalendarMapping) error
	// FailCalendars отмечает terminal sync failure без регресса версии.
	// @args контекст — отмена; job — lease/version; строка — безопасный код.
	// @return: ошибка DB/fencing.
	FailCalendars(context.Context, jobs.Job, string) error
	// AcquireCalendar сериализует продуктовый provider path одной конференции.
	// @args контекст — deadline; строка — conferenceID.
	// @return: обязательная release функция либо retryable занятость.
	AcquireCalendar(context.Context, string) (func(), error)
	// Fanout создаёт bounded личные события и persistent continuation для остальных получателей.
	// @args контекст — отмена; job — источник/lease; строка — известный event kind.
	// @return: ошибка либо ErrSkip для устаревшего события.
	Fanout(context.Context, jobs.Job, string) error
	// ScheduleReminders создаёт будущие durable jobs текущего расписания и убирает expired OAuth states bounded порцией.
	// @args контекст — цикл scheduler; offsets — конечный набор UTC интервалов.
	// @return: ошибка DB.
	ScheduleReminders(context.Context, []time.Duration) error
	// Delivery проверяет live membership, ready generation и предпочтения перед внешним send.
	// @args контекст — отмена; job — личное уведомление.
	// @return: минимальная проекция и Allowed либо ошибка.
	Delivery(context.Context, jobs.Job) (Delivery, error)
	// CompleteDelivery сохраняет единственный terminal факт канала под lease fencing.
	// @args контекст — отмена; job — lease/user/channel; строки — status и безопасный error code.
	// @return: ошибка DB/fencing.
	CompleteDelivery(context.Context, jobs.Job, string, string) error
	// ScheduleCalendarSync сохраняет bounded задания после подключения календаря.
	// @args контекст — отмена; строка — owner userID.
	// @return: ошибка enqueue.
	ScheduleCalendarSync(context.Context, string) error
}

// TokenCipher шифрует и привязывает секрет к конкретному пользователю/подключению.
type TokenCipher interface {
	// Encrypt шифрует секрет с криптографической привязкой к владельцу/назначению.
	// @args строки — plaintext и binding контекст.
	// @return: ciphertext либо ошибка.
	Encrypt(string, string) (string, error)
	// Decrypt запрещает расшифровку повреждённого либо чужого ciphertext.
	// @args строки — ciphertext и ожидаемый binding.
	// @return: plaintext только серверному потребителю либо ошибка.
	Decrypt(string, string) (string, error)
}

// Service связывает durable события с внешними адаптерами, не блокируя конференцию или запись ожиданием провайдера.
type Service struct {
	repo     Repository
	adapters d.Providers
	cipher   TokenCipher
	options  Options
	html     *template.Template
	text     *texttemplate.Template
}

// NewService проверяет URL и ресурсные пределы, затем подготавливает безопасные шаблоны.
// @args repo — постоянное хранилище; adapters — заменяемые провайдеры; cipher — независимое шифрование; options — настройки.
// @return: сервис либо ошибка некорректной конфигурации.
func NewService(repo Repository, adapters d.Providers, cipher TokenCipher, options Options) (*Service, error) {
	allNoop := adapters.Capabilities.Email == "noop" && adapters.Capabilities.Push == "noop" && adapters.Capabilities.Calendar == "noop"
	u, err := url.Parse(options.PublicURL)
	if !(allNoop && options.PublicURL == "") && (err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http")) {
		return nil, errors.New("PUBLIC_FRONTEND_URL must be an absolute HTTP(S) URL")
	}
	if options.ProviderTimeout <= 0 {
		options.ProviderTimeout = 30 * time.Second
	}
	if options.ProviderTimeout > 5*time.Minute {
		return nil, errors.New("integration timeout exceeds five minutes")
	}
	if len(options.ReminderOffsets) == 0 {
		options.ReminderOffsets = []time.Duration{24 * time.Hour, 15 * time.Minute}
	}
	if len(options.ReminderOffsets) > 8 {
		return nil, errors.New("too many reminder offsets")
	}
	seen := map[time.Duration]bool{}
	for _, v := range options.ReminderOffsets {
		if v < time.Second || v%time.Second != 0 || v > 30*24*time.Hour || seen[v] {
			return nil, errors.New("invalid reminder offsets")
		}
		seen[v] = true
	}
	h := template.Must(template.New("notification-v1").Parse(`<h1>{{.Subject}}</h1><p>{{.Title}}</p><p>{{.Body}}</p><p><a href="{{.URL}}">Открыть встречу</a></p>`))
	t := texttemplate.Must(texttemplate.New("notification-v1").Parse("{{.Subject}}\n{{.Title}}\n{{.Body}}\n{{.URL}}\n"))
	return &Service{repo: repo, adapters: adapters, cipher: cipher, options: options, html: h, text: t}, nil
}

// Capabilities возвращает только безопасные признаки настроенных каналов.
// @return: конфигурационная проекция без endpoints/secrets.
func (s *Service) Capabilities() d.Capabilities { return s.adapters.Capabilities }

// Preferences читает настройки владельца, используя opt-out внутренние/opt-in внешние defaults.
// @args ctx — отмена; userID — авторизованный пользователь.
// @return: его настройки либо ошибка.
func (s *Service) Preferences(ctx context.Context, userID string) (d.Preferences, error) {
	return s.repo.Preferences(ctx, userID)
}

// SavePreferences сохраняет только настройки текущего пользователя.
// @args ctx — отмена; userID — доверенная identity; value — boolean preferences.
// @return: ошибка сохранения.
func (s *Service) SavePreferences(ctx context.Context, userID string, value d.Preferences) error {
	value.UserID = userID
	return s.repo.SavePreferences(ctx, value)
}

// Devices возвращает проекцию регистраций без provider token/ciphertext.
// @args ctx — отмена; userID — владелец.
// @return: ограниченный список устройств либо ошибка.
func (s *Service) Devices(ctx context.Context, userID string) ([]d.Device, error) {
	return s.repo.Devices(ctx, userID, false)
}

// RegisterDevice проверяет тип устройства и шифрует sensitive токен до обращения к базе.
// @args ctx — отмена; userID — владелец; request — token от доверенного platform SDK.
// @return: безопасная регистрация либо ошибка; noop не принимает бесполезные секреты.
func (s *Service) RegisterDevice(ctx context.Context, userID string, request d.DeviceRequest) (d.Device, error) {
	if s.adapters.Capabilities.Push == "noop" || s.cipher == nil {
		return d.Device{}, apperrors.ErrUnavailable
	}
	if (request.Platform != "web" && request.Platform != "ios" && request.Platform != "android") || len(request.Token) < 8 || len(request.Token) > 8192 {
		return d.Device{}, apperrors.ErrInvalidInput
	}
	sum := sha256.Sum256([]byte(request.Token))
	fp := hex.EncodeToString(sum[:])
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(userID+":"+s.adapters.Capabilities.Push+":"+fp)).String()
	binding := userID + ":device:" + id
	encrypted, err := s.cipher.Encrypt(request.Token, binding)
	if err != nil {
		return d.Device{}, apperrors.ErrUnavailable
	}
	device := d.Device{ID: id, UserID: userID, Platform: request.Platform, Provider: s.adapters.Capabilities.Push, TokenCiphertext: encrypted, TokenFingerprint: fp}
	err = s.repo.SaveDevice(ctx, device)
	return device, err
}

// RevokeDevice отключает только собственный токен и удаляет ciphertext из активной регистрации.
// @args ctx — отмена; userID — владелец; id — UUID устройства.
// @return: ошибка авторизации/поиска/сохранения.
func (s *Service) RevokeDevice(ctx context.Context, userID, id string) error {
	return s.repo.RevokeDevice(ctx, userID, id)
}

// Connections возвращает собственные календарные подключения без credentials.
// @args ctx — отмена; userID — владелец.
// @return: список либо ошибка.
func (s *Service) Connections(ctx context.Context, userID string) ([]d.CalendarConnection, error) {
	return s.repo.Connections(ctx, userID)
}

// MockConnect включает явно демонстрационный календарь только в разрешённом локальном/test режиме.
// @args ctx — отмена; userID — владелец.
// @return: безопасное подключение либо ошибка недоступности.
func (s *Service) MockConnect(ctx context.Context, userID string) (d.CalendarConnection, error) {
	if !s.options.MockConnectAllowed || !s.adapters.Capabilities.MockConnectAllowed {
		return d.CalendarConnection{}, apperrors.ErrUnavailable
	}
	c := d.CalendarConnection{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(userID+":calendar:mock")).String(), UserID: userID, Provider: "mock", CalendarID: "primary", Status: "connected"}
	if err := s.repo.SaveConnection(ctx, c); err != nil {
		return c, err
	}
	return c, s.repo.ScheduleCalendarSync(ctx, userID)
}

// OAuthStart создаёт одноразовый state и PKCE verifier, привязанные к авторизованному пользователю.
// @args ctx — отмена; userID — владелец; provider — настроенный generic adapter.
// @return: authorization URL, state либо ошибка.
func (s *Service) OAuthStart(ctx context.Context, userID, provider string) (string, string, error) {
	if provider != "generic" || s.adapters.OAuth == nil || s.cipher == nil {
		return "", "", apperrors.ErrUnavailable
	}
	state, err := randomToken()
	if err != nil {
		return "", "", err
	}
	verifier, err := randomToken()
	if err != nil {
		return "", "", err
	}
	id := uuid.NewString()
	encrypted, err := s.cipher.Encrypt(verifier, userID+":oauth:"+id)
	if err != nil {
		return "", "", err
	}
	record := d.OAuthState{ID: id, UserID: userID, Provider: provider, StateHash: digest(state), VerifierCiphertext: encrypted, ExpiresAt: time.Now().UTC().Add(10 * time.Minute)}
	if err = s.repo.SaveOAuthState(ctx, record); err != nil {
		return "", "", err
	}
	challenge := sha256.Sum256([]byte(verifier))
	authURL, err := s.adapters.OAuth.AuthorizeURL(state, base64.RawURLEncoding.EncodeToString(challenge[:]))
	return authURL, state, err
}

// OAuthCallback одноразово проверяет state/user/provider до отправки code к token endpoint.
// @args ctx — отмена; userID/provider — доверенная identity/адаптер; code/state — короткоживущие OAuth значения.
// @return: подключение без секретов либо безопасная ошибка.
func (s *Service) OAuthCallback(ctx context.Context, userID, provider, code, state string) (d.CalendarConnection, error) {
	if provider != "generic" || s.adapters.OAuth == nil || s.cipher == nil {
		return d.CalendarConnection{}, apperrors.ErrUnavailable
	}
	if len(code) < 1 || len(code) > 4096 || len(state) != 43 {
		return d.CalendarConnection{}, apperrors.ErrInvalidInput
	}
	record, err := s.repo.TakeOAuthState(ctx, userID, provider, digest(state))
	if err != nil {
		return d.CalendarConnection{}, err
	}
	verifier, err := s.cipher.Decrypt(record.VerifierCiphertext, userID+":oauth:"+record.ID)
	if err != nil {
		return d.CalendarConnection{}, apperrors.ErrUnavailable
	}
	op, cancel := context.WithTimeout(ctx, s.options.ProviderTimeout)
	defer cancel()
	credentials, err := s.adapters.OAuth.Exchange(op, code, verifier)
	if err != nil {
		return d.CalendarConnection{}, apperrors.ErrUnavailable
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(userID+":calendar:generic")).String()
	connection := d.CalendarConnection{ID: id, UserID: userID, Provider: provider, CalendarID: "primary", Status: "connected"}
	if err = s.encryptCredentials(&connection, credentials); err != nil {
		return connection, err
	}
	if err = s.repo.SaveConnection(ctx, connection); err != nil {
		return connection, err
	}
	return connection, s.repo.ScheduleCalendarSync(ctx, userID)
}

// Disconnect сначала запрещает локальное использование и очищает ciphertext, затем пытается отозвать внешний токен.
// @args ctx — отмена; userID — владелец; id — UUID подключения.
// @return: ошибка локального отключения; внешний revoke failure не возвращает секреты и не восстанавливает доступ.
func (s *Service) Disconnect(ctx context.Context, userID, id string) error {
	items, err := s.repo.Connections(ctx, userID)
	if err != nil {
		return err
	}
	var found *d.CalendarConnection
	for _, c := range items {
		if c.ID == id {
			v := c
			found = &v
			break
		}
	}
	if found == nil {
		return apperrors.ErrNotFound
	}
	token := ""
	if found.RefreshCiphertext != "" && s.cipher != nil {
		token, _ = s.cipher.Decrypt(found.RefreshCiphertext, userID+":calendar:"+id+":refresh")
	}
	if err = s.repo.RevokeConnection(ctx, userID, id); err != nil {
		return err
	}
	if s.adapters.OAuth != nil && token != "" {
		op, cancel := context.WithTimeout(ctx, s.options.ProviderTimeout)
		defer cancel()
		_ = s.adapters.OAuth.Revoke(op, token)
	}
	return nil
}

// CalendarMappings разрешает просмотр синхронизации только организатору/соорганизатору.
// @args ctx — отмена; userID — actor; conferenceID — область встречи.
// @return: безопасные mappings либо ошибка доступа.
func (s *Service) CalendarMappings(ctx context.Context, userID, conferenceID string) ([]d.CalendarMapping, error) {
	return s.repo.CalendarMappings(ctx, userID, conferenceID)
}

// Tick создаёт durable reminder facts одной ограниченной SQL-порцией; timer на каждую встречу не используется.
// @args ctx — общий ограниченный цикл scheduler.
// @return: ошибка планирования.
func (s *Service) Tick(ctx context.Context) error {
	return s.repo.ScheduleReminders(ctx, s.options.ReminderOffsets)
}

// eventPayload хранит только ссылочную нагрузку фонового события.
type eventPayload struct {
	Event          string `json:"event"`
	NotificationID string `json:"notificationId"`
	Channel        string `json:"channel"`
}

// Handle маршрутизирует durable работу, сохраняя отдельные отказные состояния каналов.
// @args ctx — timeout worker; job — задание с lease и стабильным ключом.
// @return: provider classification, ErrSkip или ошибка постоянного хранилища.
func (s *Service) Handle(ctx context.Context, job jobs.Job) error {
	var payload eventPayload
	if json.Unmarshal(job.Payload, &payload) != nil {
		return jobs.Error{Code: "integration_payload"}
	}
	switch job.Kind {
	case "integrations.delivery":
		return s.deliver(ctx, job, payload)
	case "integrations.event":
		return s.repo.Fanout(ctx, job, payload.Event)
	case "integrations.conference":
		if err := s.repo.Fanout(ctx, job, payload.Event); err != nil {
			return err
		}
		return s.syncCalendar(ctx, job)
	case "integrations.calendar":
		return s.syncCalendar(ctx, job)
	}
	return jobs.Error{Code: "integration_kind"}
}

// FailJob сохраняет терминальный факт доставки без сообщения провайдера или transcript в журнале.
// @args ctx — финальная bounded транзакция; job — последний lease; code — безопасный код сбоя.
// @return: ошибка сохранения.
func (s *Service) FailJob(ctx context.Context, job jobs.Job, code string) error {
	if job.Kind == "integrations.delivery" {
		return s.repo.CompleteDelivery(ctx, job, "failed", code)
	}
	if job.Kind == "integrations.calendar" || job.Kind == "integrations.conference" {
		return s.repo.FailCalendars(ctx, job, code)
	}
	return nil
}

// templateData содержит только безопасную минимальную проекцию события.
type templateData struct{ Subject, Title, Body, URL string }

// RenderEmail формирует HTML с автоматическим escaping и plain-text из версии шаблона notification-v1.
// @args key/to — dedup и адрес; kind — событие; title/url — название и доверенная frontend ссылка.
// @return: письмо без transcript либо ошибка рендеринга.
func (s *Service) RenderEmail(key, to, kind, title, link string) (d.EmailMessage, error) {
	body := eventText(kind)
	data := templateData{Subject: body, Title: title, Body: body, URL: link}
	var h, t bytes.Buffer
	if err := s.html.Execute(&h, data); err != nil {
		return d.EmailMessage{}, err
	}
	if err := s.text.Execute(&t, data); err != nil {
		return d.EmailMessage{}, err
	}
	return d.EmailMessage{IdempotencyKey: key, To: to, Subject: body, HTML: h.String(), Text: t.String()}, nil
}

// eventText выбирает фиксированный локализованный текст, исключая произвольный provider error и transcript.
// @args kind — известное имя события.
// @return: краткий безопасный текст.
func eventText(kind string) string {
	switch kind {
	case "conference.invited":
		return "Приглашение на встречу"
	case "conference.rescheduled":
		return "Время встречи изменилось"
	case "conference.cancelled":
		return "Встреча отменена"
	case "conference.soon":
		return "Встреча скоро начнётся"
	case "recording.ready":
		return "Запись встречи готова"
	case "transcript.ready":
		return "Расшифровка встречи готова"
	case "summary.ready":
		return "Итоги встречи готовы"
	case "processing.failed":
		return "Обработка встречи не завершилась"
	default:
		return "Обновление встречи"
	}
}

// deliver повторно проверяет настройки и live authorization перед каждым внешним вызовом.
// @args ctx — timeout; job — сохранённое задание; payload — канал и ссылка уведомления.
// @return: ошибка провайдера/базы или ErrSkip при отключении.
func (s *Service) deliver(ctx context.Context, job jobs.Job, payload eventPayload) error {
	delivery, err := s.repo.Delivery(ctx, job)
	if err != nil {
		return err
	}
	skip := !delivery.Allowed || !delivery.Preferences.Allows(delivery.Notification.Type)
	if payload.Channel == "email" {
		skip = skip || !delivery.Preferences.Email || s.adapters.Capabilities.Email == "noop"
	} else if payload.Channel == "push" {
		skip = skip || !delivery.Preferences.Push || s.adapters.Capabilities.Push == "noop"
	} else {
		return jobs.Error{Code: "delivery_channel"}
	}
	if skip {
		if err = s.repo.CompleteDelivery(ctx, job, "skipped", ""); err != nil {
			return err
		}
		return jobs.ErrSkip
	}
	conference, err := s.repo.Conference(ctx, job.ConferenceID)
	if err != nil {
		return err
	}
	link := strings.TrimRight(s.options.PublicURL, "/") + "/conferences/" + url.PathEscape(conference.ID)
	if delivery.Notification.Payload.RecordingID != "" && (delivery.Notification.Type == "transcript.ready" || delivery.Notification.Type == "summary.ready") {
		query := url.Values{"recording": {delivery.Notification.Payload.RecordingID}, "tab": {"transcript"}}
		if delivery.Notification.Type == "summary.ready" {
			query.Set("tab", "summary")
		}
		link += "?" + query.Encode()
	}
	op, cancel := context.WithTimeout(ctx, s.options.ProviderTimeout)
	defer cancel()
	if payload.Channel == "email" {
		message, err := s.RenderEmail(job.DedupKey, delivery.Email, delivery.Notification.Type, conference.Title, link)
		if err != nil {
			return err
		}
		if err = s.adapters.Email.Send(op, message); err != nil {
			return err
		}
	} else {
		devices, err := s.repo.Devices(ctx, delivery.Notification.UserID, true)
		if err != nil {
			return err
		}
		if len(devices) == 0 {
			if err = s.repo.CompleteDelivery(ctx, job, "skipped", ""); err != nil {
				return err
			}
			return jobs.ErrSkip
		}
		for _, device := range devices {
			if s.cipher == nil {
				return jobs.Error{Code: "provider_encryption_disabled"}
			}
			token, err := s.cipher.Decrypt(device.TokenCiphertext, device.UserID+":device:"+device.ID)
			if err != nil {
				return jobs.Error{Code: "device_token_invalid"}
			}
			if err = s.adapters.Push.Send(op, d.PushMessage{IdempotencyKey: job.DedupKey + ":" + device.ID, Token: token, Title: conference.Title, Body: eventText(delivery.Notification.Type), URL: link}); err != nil {
				return err
			}
		}
	}
	return s.repo.CompleteDelivery(ctx, job, "delivered", "")
}

// syncCalendar сохраняет mapping с проверкой lease после provider call; актуальная версия расписания вытесняет устаревшие jobs.
// @args ctx — timeout; job — версия конференции и текущий lease.
// @return: ошибка или ErrSkip при устаревшем/отключённом событии.
func (s *Service) syncCalendar(ctx context.Context, job jobs.Job) error {
	if s.adapters.Capabilities.Calendar == "noop" {
		return jobs.ErrSkip
	}
	release, err := s.repo.AcquireCalendar(ctx, job.ConferenceID)
	if err != nil {
		return err
	}
	defer release()
	conference, err := s.repo.Conference(ctx, job.ConferenceID)
	if err != nil {
		return err
	}
	if job.Version != conference.IntegrationVersion {
		return jobs.ErrSkip
	}
	connections, err := s.repo.Connections(ctx, conference.OwnerID)
	if err != nil {
		return err
	}
	mappings, err := s.repo.MappingsForSync(ctx, conference.ID)
	if err != nil {
		return err
	}
	for _, connection := range connections {
		if connection.Status != "connected" {
			continue
		}
		if (connection.Provider == "mock" && s.adapters.Capabilities.Calendar != "mock") || (connection.Provider != "mock" && s.adapters.Capabilities.Calendar != "http") {
			continue
		}
		mapping := d.CalendarMapping{ConferenceID: conference.ID, ConnectionID: connection.ID, Provider: connection.Provider, ExternalCalendarID: connection.CalendarID, SourceVersion: job.Version, SyncStatus: "pending"}
		for _, m := range mappings {
			if m.ConnectionID == connection.ID {
				mapping.ID = m.ID
				mapping.ExternalEventID = m.ExternalEventID
				break
			}
		}
		if conference.Status != "cancelled" && conference.ScheduledAt == nil {
			continue
		}
		credentials, err := s.connectionCredentials(ctx, &connection)
		if err != nil {
			return err
		}
		duration := 60
		if conference.PlannedDurationMin != nil {
			duration = *conference.PlannedDurationMin
		}
		event := d.CalendarEvent{IdempotencyKey: "calendar:" + conference.ID + ":" + connection.ID, ID: mapping.ExternalEventID, CalendarID: connection.CalendarID, Title: conference.Title, SourceVersion: job.Version, JoinURL: strings.TrimRight(s.options.PublicURL, "/") + "/i/" + url.PathEscape(conference.InviteCode)}
		if conference.ScheduledAt != nil {
			event.StartsAt = conference.ScheduledAt.UTC()
			event.EndsAt = event.StartsAt.Add(time.Duration(duration) * time.Minute)
		}
		op, cancel := context.WithTimeout(ctx, s.options.ProviderTimeout)
		if conference.Status == "cancelled" {
			if event.ID != "" {
				err = s.adapters.Calendar.CancelEvent(op, credentials, event)
			}
			mapping.SyncStatus = "cancelled"
		} else if event.ID == "" {
			var result d.CalendarEvent
			result, err = s.adapters.Calendar.CreateEvent(op, credentials, event)
			mapping.ExternalEventID = result.ID
			// Idempotent create после потерянного response может вернуть event прежней версии;
			// новый schedule должен обновить тот же ID, а не объявить старые данные synced.
			if err == nil && (result.SourceVersion != job.Version || !result.StartsAt.Equal(event.StartsAt) || !result.EndsAt.Equal(event.EndsAt) || result.Title != event.Title || result.JoinURL != event.JoinURL) {
				event.ID = result.ID
				event.IdempotencyKey += ":" + strconv.FormatInt(job.Version, 10)
				_, err = s.adapters.Calendar.UpdateEvent(op, credentials, event)
			}
			mapping.SyncStatus = "synced"
		} else {
			event.IdempotencyKey += ":" + strconv.FormatInt(job.Version, 10)
			_, err = s.adapters.Calendar.UpdateEvent(op, credentials, event)
			mapping.SyncStatus = "synced"
		}
		cancel()
		if err != nil {
			mapping.SyncStatus = "failed"
			_ = s.repo.SaveMapping(ctx, job, mapping)
			return err
		}
		now := time.Now().UTC()
		mapping.LastSyncedAt = &now
		if err = s.repo.SaveMapping(ctx, job, mapping); err != nil {
			return err
		}
	}
	return nil
}

// encryptCredentials связывает access/refresh ciphertext с разными назначениями одного подключения.
// @args connection — сохраняемое подключение; credentials — новый provider response.
// @return: ошибка шифрования.
func (s *Service) encryptCredentials(connection *d.CalendarConnection, credentials d.CalendarCredentials) error {
	if s.cipher == nil {
		return apperrors.ErrUnavailable
	}
	binding := connection.UserID + ":calendar:" + connection.ID
	access, err := s.cipher.Encrypt(credentials.AccessToken, binding+":access")
	if err != nil {
		return err
	}
	refresh := ""
	if credentials.RefreshToken != "" {
		refresh, err = s.cipher.Encrypt(credentials.RefreshToken, binding+":refresh")
		if err != nil {
			return err
		}
	}
	connection.AccessCiphertext = access
	connection.RefreshCiphertext = refresh
	connection.ExpiresAt = credentials.ExpiresAt
	connection.Scopes = credentials.Scopes
	return nil
}

// connectionCredentials расшифровывает credentials и обновляет истекающий токен до calendar operation.
// @args ctx — timeout; connection — сохранённое подключение текущего владельца.
// @return: серверные credentials либо ошибка, без записи токенов в logs.
func (s *Service) connectionCredentials(ctx context.Context, connection *d.CalendarConnection) (d.CalendarCredentials, error) {
	if connection.Provider == "mock" {
		return d.CalendarCredentials{}, nil
	}
	if s.cipher == nil {
		return d.CalendarCredentials{}, jobs.Error{Code: "provider_encryption_disabled"}
	}
	binding := connection.UserID + ":calendar:" + connection.ID
	access, err := s.cipher.Decrypt(connection.AccessCiphertext, binding+":access")
	if err != nil {
		return d.CalendarCredentials{}, jobs.Error{Code: "calendar_token_invalid"}
	}
	refresh := ""
	if connection.RefreshCiphertext != "" {
		refresh, err = s.cipher.Decrypt(connection.RefreshCiphertext, binding+":refresh")
		if err != nil {
			return d.CalendarCredentials{}, jobs.Error{Code: "calendar_token_invalid"}
		}
	}
	credentials := d.CalendarCredentials{AccessToken: access, RefreshToken: refresh, ExpiresAt: connection.ExpiresAt, Scopes: connection.Scopes}
	if credentials.ExpiresAt != nil && credentials.ExpiresAt.Before(time.Now().Add(time.Minute)) {
		if s.adapters.OAuth == nil || refresh == "" {
			return credentials, jobs.Error{Code: "calendar_reconnect_required"}
		}
		credentials, err = s.adapters.OAuth.Refresh(ctx, refresh)
		if err != nil {
			return credentials, err
		}
		oldRefresh := connection.RefreshCiphertext
		if err = s.encryptCredentials(connection, credentials); err != nil {
			return credentials, err
		}
		if err = s.repo.RefreshConnection(ctx, *connection, oldRefresh); err != nil {
			return credentials, err
		}
	}
	return credentials, nil
}

// randomToken генерирует достаточную криптографическую энтропию для state/PKCE.
// @return: 43-символьный base64url token либо ошибка источника случайности.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// digest сохраняет лишь хеш одноразового state, не его plaintext.
// @args value — случайный OAuth state.
// @return: SHA-256 в hex.
func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

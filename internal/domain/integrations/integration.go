package integrations

import (
	"context"
	"time"
)

// Preferences определяет категории личных уведомлений и разрешение внешних каналов; email/push по умолчанию выключены.
type Preferences struct {
	UserID     string    `json:"-" gorm:"type:uuid;primaryKey"`
	Invitation bool      `json:"invitation"`
	Reminder   bool      `json:"reminder"`
	Recording  bool      `json:"recording"`
	Summary    bool      `json:"summary"`
	Email      bool      `json:"email"`
	Push       bool      `json:"push"`
	UpdatedAt  time.Time `json:"-"`
}

// TableName привязывает пользовательские настройки к постоянной таблице.
// @return: имя таблицы PostgreSQL.
func (Preferences) TableName() string { return "notification_preferences" }

// DefaultPreferences включает полезные внутренние события, но не передаёт данные внешним провайдерам без согласия.
// @args userID — владелец настроек.
// @return: первоначальные настройки пользователя.
func DefaultPreferences(userID string) Preferences {
	return Preferences{UserID: userID, Invitation: true, Reminder: true, Recording: true, Summary: true}
}

// Allows проверяет разрешённую категорию события без изменения настроек.
// @args kind — техническое имя события.
// @return: true для включённой категории; неизвестные события запрещены.
func (p Preferences) Allows(kind string) bool {
	switch kind {
	case "conference.invited", "conference.rescheduled", "conference.cancelled":
		return p.Invitation
	case "conference.soon":
		return p.Reminder
	case "recording.ready":
		return p.Recording
	case "transcript.ready", "summary.ready":
		return p.Summary
	case "admission.decided", "processing.failed":
		return true
	}
	return false
}

// Device хранит регистрацию устройства; зашифрованный токен и fingerprint никогда не входят в API-ответ.
type Device struct {
	ID               string     `json:"id" gorm:"type:uuid;primaryKey"`
	UserID           string     `json:"-"`
	Platform         string     `json:"platform"`
	Provider         string     `json:"provider"`
	TokenCiphertext  string     `json:"-"`
	TokenFingerprint string     `json:"-"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	RevokedAt        *time.Time `json:"revokedAt,omitempty"`
	DisabledAt       *time.Time `json:"disabledAt,omitempty"`
}

// TableName возвращает имя безопасного хранилища регистраций push.
// @return: имя таблицы.
func (Device) TableName() string { return "push_devices" }

// DeviceRequest содержит токен от доверенного приложения устройства; произвольные URL провайдера не принимаются.
type DeviceRequest struct {
	Platform string `json:"platform"`
	Token    string `json:"token"`
}

// CalendarConnection связывает внешний календарь с владельцем; OAuth-секреты доступны только серверу.
type CalendarConnection struct {
	ID                string     `json:"id" gorm:"type:uuid;primaryKey"`
	UserID            string     `json:"-"`
	Provider          string     `json:"provider"`
	CalendarID        string     `json:"calendarId"`
	Status            string     `json:"status"`
	AccessCiphertext  string     `json:"-"`
	RefreshCiphertext string     `json:"-"`
	Scopes            string     `json:"-"`
	ExpiresAt         *time.Time `json:"expiresAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

// TableName возвращает имя таблицы подключений, отдельной от независимого от поставщика расписания.
// @return: имя таблицы.
func (CalendarConnection) TableName() string { return "calendar_connections" }

// CalendarMapping сохраняет единственный внешний event для пары конференция/подключение.
type CalendarMapping struct {
	ID                 string     `json:"-" gorm:"type:uuid;primaryKey"`
	ConferenceID       string     `json:"conferenceId"`
	ConnectionID       string     `json:"-"`
	Provider           string     `json:"provider"`
	ExternalCalendarID string     `json:"externalCalendarId"`
	ExternalEventID    string     `json:"externalEventId"`
	SyncStatus         string     `json:"syncStatus"`
	SourceVersion      int64      `json:"-"`
	LastSyncedAt       *time.Time `json:"lastSyncedAt,omitempty"`
}

// TableName возвращает имя таблицы соответствий внешних календарей.
// @return: имя таблицы.
func (CalendarMapping) TableName() string { return "calendar_event_mappings" }

// OAuthState сохраняет короткоживущее одноразовое состояние и зашифрованное проверочное значение PKCE.
type OAuthState struct {
	ID                 string
	UserID             string
	Provider           string
	StateHash          string
	VerifierCiphertext string
	ExpiresAt          time.Time
	UsedAt             *time.Time
}

// TableName возвращает имя таблицы одноразовых запросов OAuth.
// @return: имя таблицы.
func (OAuthState) TableName() string { return "calendar_oauth_states" }

// EmailMessage несёт минимальные данные письма; идентификатор стабилен для повторной доставки.
type EmailMessage struct {
	IdempotencyKey string `json:"idempotencyKey"`
	To             string `json:"to"`
	Subject        string `json:"subject"`
	HTML           string `json:"html"`
	Text           string `json:"text"`
}

// PushMessage не содержит расшифровок встреч и содержит лишь ссылку и безопасный текст события.
type PushMessage struct {
	IdempotencyKey string `json:"idempotencyKey"`
	Token          string `json:"token"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	URL            string `json:"url"`
}

// CalendarEvent представляет внешний event без импорта SDK конкретного производителя.
type CalendarEvent struct {
	IdempotencyKey string    `json:"idempotencyKey"`
	ID             string    `json:"id,omitempty"`
	CalendarID     string    `json:"calendarId"`
	Title          string    `json:"title"`
	JoinURL        string    `json:"joinUrl"`
	StartsAt       time.Time `json:"startsAt"`
	EndsAt         time.Time `json:"endsAt"`
	Attendees      []string  `json:"attendees,omitempty"`
	SourceVersion  int64     `json:"sourceVersion"`
}

// CalendarCredentials передаёт OAuth-токен только внутри сервера, вне публичного JSON и журналов.
type CalendarCredentials struct {
	AccessToken  string     `json:"-"`
	RefreshToken string     `json:"-"`
	ExpiresAt    *time.Time `json:"-"`
	Scopes       string     `json:"-"`
}

// EmailProvider отделяет прикладную доставку от конкретного сервиса и требует идемпотентности по ключу сообщения.
type EmailProvider interface {
	// Send доставляет письмо, дедуплицируя повтор по IdempotencyKey.
	// @args контекст — срок выполнения; письмо — получатель, HTML с экранированием и обычный текст.
	// @return: классифицированная ошибка либо nil после приёма провайдером.
	Send(context.Context, EmailMessage) error
}

// PushProvider отделяет платформенную отправку push от SDK конкретного поставщика.
type PushProvider interface {
	// Send отправляет минимальное push-событие конкретному устройству.
	// @args контекст — срок выполнения; сообщение — секретный токен, текст, ссылка и ключ дедупликации.
	// @return: классифицированная ошибка либо nil.
	Send(context.Context, PushMessage) error
}

// CalendarProvider задаёт контракт создания, обновления, отмены и чтения; CreateEvent соблюдает идемпотентность по ключу.
type CalendarProvider interface {
	// CreateEvent создаёт не более одного события для стабильного ключа идемпотентности.
	// @args контекст — срок выполнения; credentials — серверное разрешение OAuth; event — расписание и версия.
	// @return: внешний event со стабильным ID либо ошибка.
	CreateEvent(context.Context, CalendarCredentials, CalendarEvent) (CalendarEvent, error)
	// UpdateEvent меняет существующий event; более старая SourceVersion не должна вытеснять новую.
	// @args контекст — срок выполнения; credentials — серверное разрешение; event — прежний ID и новое расписание.
	// @return: актуальный event либо ошибка.
	UpdateEvent(context.Context, CalendarCredentials, CalendarEvent) (CalendarEvent, error)
	// CancelEvent идемпотентно отменяет ранее сопоставленный event.
	// @args контекст — срок выполнения; credentials — серверное разрешение; event — существующий ID.
	// @return: безопасная ошибка отмены либо nil.
	CancelEvent(context.Context, CalendarCredentials, CalendarEvent) error
	// GetEvent читает событие для восстановления либо диагностики соответствия внешнего ресурса.
	// @args контекст — срок выполнения; credentials — серверное разрешение; event — внешний ID.
	// @return: актуальный event либо ошибка.
	GetEvent(context.Context, CalendarCredentials, CalendarEvent) (CalendarEvent, error)
}

// OAuthProvider реализует серверную авторизацию, refresh и revoke с ограниченными настроенными scopes.
type OAuthProvider interface {
	// AuthorizeURL формирует перенаправление без секрета клиента в строке запроса.
	// @args state — одноразовое состояние; challenge — проверочное значение PKCE по методу S256.
	// @return: authorization URL либо ошибка.
	AuthorizeURL(string, string) (string, error)
	// Exchange выполняет обмен кода авторизации на токены на сервере.
	// @args контекст — срок выполнения; code — одноразовый код; verifier — секрет PKCE.
	// @return: credentials, которые должны быть зашифрованы перед сохранением, либо ошибка.
	Exchange(context.Context, string, string) (CalendarCredentials, error)
	// Refresh обновляет токен доступа и обрабатывает смену токена обновления.
	// @args контекст — срок выполнения; token — серверный токен обновления.
	// @return: новые credentials либо ошибка.
	Refresh(context.Context, string) (CalendarCredentials, error)
	// Revoke отзывает внешнее разрешение, не изменяя локальную политику подключения.
	// @args контекст — срок выполнения; token — токен доступа или обновления, хранящийся только на сервере.
	// @return: ошибка провайдера либо nil.
	Revoke(context.Context, string) error
}

// Capabilities описывает реально настроенные адаптеры; noop не выдаётся за успешную внешнюю доставку.
type Capabilities struct {
	Email                   string `json:"email"`
	Push                    string `json:"push"`
	Calendar                string `json:"calendar"`
	CalendarOAuthConfigured bool   `json:"calendarOAuthConfigured"`
	MockConnectAllowed      bool   `json:"mockConnectAllowed"`
}

// Providers собирает заменяемые адаптеры и доступные способы подключения.
type Providers struct {
	Email        EmailProvider
	Push         PushProvider
	Calendar     CalendarProvider
	OAuth        OAuthProvider
	Capabilities Capabilities
}

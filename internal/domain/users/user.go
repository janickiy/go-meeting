package users

import (
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

// User сохраняет учётную запись и хеш пароля; публичное представление формируется отдельно.
// @params
//   - ID: уникальный идентификатор данной сущности.
//   - Email: адрес электронной почты пользователя.
//   - PasswordHash: защищённый хеш пароля, который не передаётся в публичном представлении.
//   - DisplayName: имя пользователя, отображаемое участникам встречи.
//   - CreatedAt: время создания значения.
//   - UpdatedAt: время последнего сохранённого изменения.
type User struct {
	ID                string    `gorm:"type:uuid;primaryKey" json:"-"`
	Email             string    `json:"-"`
	PasswordHash      string    `gorm:"column:password_hash" json:"-"`
	DisplayName       *string   `gorm:"column:display_name" json:"-"`
	IsAdmin           bool      `gorm:"column:is_admin" json:"-"`
	GuestConferenceID *string   `gorm:"column:guest_conference_id" json:"-"`
	CreatedAt         time.Time `json:"-"`
	UpdatedAt         time.Time `json:"-"`
}

const (
	MinPasswordCharacters = 8
	MaxPasswordCharacters = 128
)

// TableName возвращает точное имя таблицы для GORM, чтобы модель не зависела от автоматического образования имени.
//
// @return:
//   - результат 1 (string): имя таблицы, используемое ORM.
func (User) TableName() string { return "users" }

// View представляет безопасную публичную проекцию доменной модели для API.
// @params:
//   - ID: уникальный идентификатор данной сущности.
//   - Email: адрес электронной почты пользователя.
//   - DisplayName: имя пользователя, отображаемое участникам встречи.
//   - CreatedAt: время создания значения.
//   - UpdatedAt: время последнего сохранённого изменения.
type View struct {
	ID                string    `json:"id"`
	Email             string    `json:"email"`
	DisplayName       *string   `json:"displayName"`
	IsAdmin           bool      `json:"isAdmin"`
	GuestConferenceID *string   `json:"guestConferenceId,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// View собирает публичное представление модели для ответа API.
//
// @return:
//   - результат 1 (View): значение, подготовленное операцией для вызывающей стороны.
func (u User) View() View {
	email := u.Email
	if u.GuestConferenceID != nil {
		email = ""
	}
	return View{ID: u.ID, Email: email, DisplayName: u.DisplayName, IsAdmin: u.IsAdmin, GuestConferenceID: u.GuestConferenceID, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt}
}

// ParticipantName выбирает отображаемое имя пользователя для сохранённого членства в конференции.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func (u User) ParticipantName() string {
	if u.DisplayName != nil && *u.DisplayName != "" {
		return *u.DisplayName
	}
	return "Participant"
}

// RegisterRequest передаёт адрес, пароль и необязательное имя для регистрации.
// @params:
//   - Email: адрес электронной почты пользователя.
//   - Password: открытый пароль для хеширования или проверки; не предназначен для журналирования.
//   - DisplayName: имя пользователя, отображаемое участникам встречи.
type RegisterRequest struct {
	Email       string  `json:"email"`
	Password    string  `json:"password"`
	DisplayName *string `json:"displayName"`
}

// UpdateProfileRequest разрешает изменение только отображаемого имени текущей учётной записи.
type UpdateProfileRequest struct {
	DisplayName string `json:"displayName"`
}

// NormalizeDisplayName принимает непустое имя из 1–100 Unicode символов без управляющих знаков.
func NormalizeDisplayName(value string) (string, error) {
	name := strings.TrimSpace(value)
	length := utf8.RuneCountInString(name)
	if !utf8.ValidString(name) || length < 1 || length > 100 {
		return "", apperrors.New(apperrors.ErrInvalidInput, "displayName must contain 1 to 100 characters")
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return "", apperrors.New(apperrors.ErrInvalidInput, "displayName must not contain control characters")
		}
	}
	return name, nil
}

// LoginRequest передаёт учётные данные запроса входа.
// @params:
//   - Email: адрес электронной почты пользователя.
//   - Password: открытый пароль для хеширования или проверки; не предназначен для журналирования.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse возвращает результат входа, токен и безопасные сведения пользователя.
// @params:
//   - Status: состояние ресурса, ответа или фильтра выборки.
//   - AccessToken: подписанный токен учётной записи.
//   - TokenType: схема Bearer заголовка авторизации.
//   - ExpiresIn: срок действия разрешения в секундах.
//   - User: пользователь либо его идентификатор, определяющий область доступа.
type LoginResponse struct {
	Status      string `json:"status"`
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	ExpiresIn   int    `json:"expiresIn"`
	User        View   `json:"user"`
}

// NormalizeEmail обрезает пробелы и приводит адрес электронной почты к нижнему регистру.
//
// @args
//   - value (string): значение для проверки, нормализации или преобразования.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func NormalizeEmail(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

// ValidateEmail проверяет синтаксис адреса электронной почты перед регистрацией или поиском пользователя.
//
// @args
//   - value (string): значение для проверки, нормализации или преобразования.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func ValidateEmail(value string) error {
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value || len(value) > 254 || !strings.Contains(value, "@") {
		return apperrors.New(apperrors.ErrInvalidInput, "email must be a valid address, up to 254 bytes")
	}
	return nil
}

// NormalizeRegister нормализует данные регистрации и проверяет адрес, длину пароля и отображаемое имя.
//
// @args
//   - request (RegisterRequest): входные параметры соответствующего прикладного запроса.
//
// @return:
//   - результат 1 (RegisterRequest): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func NormalizeRegister(request RegisterRequest) (RegisterRequest, error) {
	request.Email = NormalizeEmail(request.Email)
	if err := ValidateEmail(request.Email); err != nil {
		return request, err
	}
	length := utf8.RuneCountInString(request.Password)
	if !utf8.ValidString(request.Password) || length < MinPasswordCharacters || length > MaxPasswordCharacters {
		return request, apperrors.New(apperrors.ErrInvalidInput, "password must contain 8 to 128 characters")
	}
	if request.DisplayName != nil {
		name := strings.TrimSpace(*request.DisplayName)
		if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 {
			return request, apperrors.New(apperrors.ErrInvalidInput, "displayName must be at most 100 characters")
		}
		request.DisplayName = nil
		if name != "" {
			request.DisplayName = &name
		}
	}
	return request, nil
}

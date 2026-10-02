package auth

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/users"
)

// userRepository задаёт контракт зависимого компонента userRepository в авторизации и учётных записях пользователей; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - Create: операция создание с контрактом, описанным у метода.
//   - GetByID: операция получение By ID с контрактом, описанным у метода.
//   - GetByEmail: операция получение By Email с контрактом, описанным у метода.
type userRepository interface {
	// Create создаёт новое состояние ресурсов компонента по переданным параметрам.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (users.User): создаваемая учётная запись с уже вычисленным хешем пароля.
	//
	// @return:
	//   - результат 1 (users.User): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Create(context.Context, users.User) (users.User, error)
	// GetByID читает учётную запись по её идентификатору.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): UUID искомого пользователя.
	//
	// @return:
	//   - результат 1 (users.User): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	GetByID(context.Context, string) (users.User, error)
	// GetByEmail читает учётную запись по нормализованному адресу электронной почты.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): адрес электронной почты пользователя.
	//
	// @return:
	//   - результат 1 (users.User): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	GetByEmail(context.Context, string) (users.User, error)
}

// passwordHasher задаёт контракт зависимого компонента passwordHasher в авторизации и учётных записях пользователей; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params:
//   - Hash: операция Hash с контрактом, описанным у метода.
//   - Verify: операция Verify с контрактом, описанным у метода.
type passwordHasher interface {
	// Hash вычисляет защищённый хеш пароля для сохранения вместо открытого текста.
	//
	// @args
	//   - аргумент 1 (string): открытый пароль для хеширования или проверки; не предназначен для журналирования.
	//
	// @return:
	//   - результат 1 (string): строка Argon2id, содержащая версию, параметры, соль и хеш пароля.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Hash(string) (string, error)
	// Verify сравнивает открытый пароль с защищённым хешем.
	//
	// @args
	//   - аргумент 1 (string): открытый пароль для хеширования или проверки; не предназначен для журналирования.
	//   - аргумент 2 (string): сохранённая строка Argon2id с параметрами, солью и ожидаемым хешем.
	//
	// @return:
	//   - результат 1 (bool): true при совпадении пароля с хешем; false при несовпадении.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Verify(string, string) (bool, error)
}

// tokenIssuer задаёт контракт зависимого компонента tokenIssuer в авторизации и учётных записях пользователей; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params:
//   - Issue: операция Issue с контрактом, описанным у метода.
type tokenIssuer interface {
	// Issue выпускает подписанный JWT пользователя с настроенным сроком действия.
	//
	// @args
	//   - аргумент 1 (string): идентификатор пользователя, для которого выполняется операция.
	//
	// @return:
	//   - результат 1 (string): подписанный JWT авторизации указанного пользователя.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Issue(string) (string, error)
}

// Service объединяет зависимости прикладного сценария и координирует его операции.
// @params:
//   - repository: хранилище постоянных данных прикладного сценария.
//   - passwords: сервис хеширования и проверки паролей.
//   - tokens: сервис выпуска или проверки JWT авторизации.
//   - dummyHash: хеш фиктивного пароля для выравнивания времени проверки неизвестной и существующей учётной записи.
type Service struct {
	repository userRepository
	passwords  passwordHasher
	tokens     tokenIssuer
	dummyHash  string
}

// NewService создаёт и связывает зависимости компонента Service, используемого в авторизации и учётных записях пользователей.
//
// @args
//   - repository (userRepository): хранилище постоянных данных прикладного сценария.
//   - passwords (passwordHasher): сервис хеширования и проверки паролей.
//   - tokens (tokenIssuer): сервис выпуска или проверки JWT авторизации.
//
// @return:
//   - результат 1 (*Service): созданный компонент с переданными зависимостями.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func NewService(repository userRepository, passwords passwordHasher, tokens tokenIssuer) (*Service, error) {
	dummy, err := passwords.Hash("dummy-password-for-login-timing")
	if err != nil {
		return nil, fmt.Errorf("initialize authentication: %w", err)
	}
	return &Service{repository: repository, passwords: passwords, tokens: tokens, dummyHash: dummy}, nil
}

// Register проверяет данные регистрации, хеширует пароль и создаёт новую учётную запись.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - request (users.RegisterRequest): входные параметры соответствующего прикладного запроса.
//
// @return:
//   - результат 1 (users.View): публичные сведения созданного пользователя без пароля и его хеша.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Register(ctx context.Context, request users.RegisterRequest) (users.View, error) {
	request, err := users.NormalizeRegister(request)
	if err != nil {
		return users.View{}, err
	}
	hash, err := s.passwords.Hash(request.Password)
	if err != nil {
		return users.View{}, err
	}
	if err := ctx.Err(); err != nil {
		return users.View{}, err
	}
	user, err := s.repository.Create(ctx, users.User{ID: uuid.NewString(), Email: request.Email, PasswordHash: hash, DisplayName: request.DisplayName})
	if err != nil {
		return users.View{}, err
	}
	return user.View(), nil
}

// Login проверяет учётные данные и возвращает безопасные сведения пользователя и токен входа.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - request (users.LoginRequest): входные параметры соответствующего прикладного запроса.
//
// @return:
//   - результат 1 (users.LoginResponse): токен авторизации, его срок действия и публичные сведения пользователя.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Login(ctx context.Context, request users.LoginRequest) (users.LoginResponse, error) {
	request.Email = users.NormalizeEmail(request.Email)
	// Старые учётные записи могут иметь менее восьми символов из-за прежней проверки длины в байтах.
	// Apply the new minimum only at registration; never reject a valid existing password.
	if users.ValidateEmail(request.Email) != nil || request.Password == "" || !utf8.ValidString(request.Password) || utf8.RuneCountInString(request.Password) > users.MaxPasswordCharacters {
		return users.LoginResponse{}, invalidCredentials()
	}
	user, err := s.repository.GetByEmail(ctx, request.Email)
	missing := errors.Is(err, apperrors.ErrNotFound)
	if err != nil && !missing {
		return users.LoginResponse{}, err
	}
	hash := user.PasswordHash
	if missing {
		hash = s.dummyHash
	}
	valid, err := s.passwords.Verify(request.Password, hash)
	if err != nil {
		return users.LoginResponse{}, err
	}
	if missing || !valid {
		return users.LoginResponse{}, invalidCredentials()
	}
	if err := ctx.Err(); err != nil {
		return users.LoginResponse{}, err
	}
	token, err := s.tokens.Issue(user.ID)
	if err != nil {
		return users.LoginResponse{}, err
	}
	return users.LoginResponse{Status: "success", AccessToken: token, TokenType: "Bearer", ExpiresIn: 3600, User: user.View()}, nil
}

// Me читает публичные сведения текущего авторизованного пользователя.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//
// @return:
//   - результат 1 (users.View): публичные сведения текущего пользователя без пароля и его хеша.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Me(ctx context.Context, userID string) (users.View, error) {
	user, err := s.repository.GetByID(ctx, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return users.View{}, apperrors.ErrUnauthorized
	}
	if err != nil {
		return users.View{}, err
	}
	return user.View(), nil
}

// invalidCredentials создаёт одинаковую безопасную ошибку для неизвестного пользователя и неверного пароля.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func invalidCredentials() error {
	return apperrors.New(apperrors.ErrUnauthorized, "invalid email or password")
}

package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// UserRepository реализует постоянное хранение ресурсов компонента через GORM.
//   - db: подключение или текущая транзакция GORM, задающая контекст доступа к базе.
type UserRepository struct{ db *gorm.DB }

// NewUserRepository создаёт и связывает зависимости компонента UserRepository, используемого в постоянном хранении данных PostgreSQL.
//
// @args
//   - db (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//
// @return:
//   - результат 1 (*UserRepository): созданный компонент с переданными зависимостями.
func NewUserRepository(db *gorm.DB) *UserRepository {
	// An INSERT failure must never cause GORM to log password_hash parameters.
	return &UserRepository{db: db.Session(&gorm.Session{Logger: logger.Discard})}
}

// Create создаёт новое состояние ресурсов компонента по переданным параметрам.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - user (users.User): пользователь либо его идентификатор, определяющий область доступа.
//
// @return:
//   - результат 1 (users.User): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *UserRepository) Create(ctx context.Context, user users.User) (users.User, error) {
	err := r.db.WithContext(ctx).Create(&user).Error
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
		return users.User{}, apperrors.New(apperrors.ErrConflict, "email is already registered")
	}
	return user, err
}

// GetByID читает учётную запись по её идентификатору.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (users.User): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *UserRepository) GetByID(ctx context.Context, id string) (users.User, error) {
	var user users.User
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&user).Error
	return user, mapNotFound(err)
}

// GetByEmail читает учётную запись по нормализованному адресу электронной почты.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - email (string): адрес электронной почты пользователя.
//
// @return:
//   - результат 1 (users.User): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (users.User, error) {
	var user users.User
	err := r.db.WithContext(ctx).Where("email = ?", email).Take(&user).Error
	return user, mapNotFound(err)
}

// UpdateDisplayName меняет только имя учётной записи из проверенного JWT subject.
func (r *UserRepository) UpdateDisplayName(ctx context.Context, userID, name string) (users.User, error) {
	result := r.db.WithContext(ctx).Model(&users.User{}).Where("id = ?", userID).Update("display_name", name)
	if result.Error != nil {
		return users.User{}, result.Error
	}
	if result.RowsAffected == 0 {
		return users.User{}, apperrors.ErrNotFound
	}
	return r.GetByID(ctx, userID)
}

// mapNotFound преобразует отсутствие строки GORM в принятую приложением ошибку отсутствующего ресурса.
//
// @args
//   - err (error): ошибка, которую необходимо классифицировать, сохранить или вернуть клиенту.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.ErrNotFound
	}
	return err
}

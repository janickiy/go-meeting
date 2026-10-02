package postgres

import (
	"context"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

// LegacyConferenceAllowed ограничивает старые маршруты записи, чтобы они не управляли защищённой записью конференции.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *RecordRepository) LegacyConferenceAllowed(ctx context.Context, id string) error {
	var count int64
	if err := r.db.WithContext(ctx).Table("conferences").Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return apperrors.New(apperrors.ErrForbidden, "use authenticated conference recording endpoints")
	}
	return nil
}

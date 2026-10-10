package postgres

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ConferenceRepository реализует постоянное хранение конференций и членств участников через GORM.
//   - db: подключение или текущая транзакция GORM, задающая контекст доступа к базе.
type ConferenceRepository struct{ db *gorm.DB }

// NewConferenceRepository создаёт и связывает зависимости компонента ConferenceRepository, используемого в жизненном цикле конференций и правах участников.
//
// @args
//   - db (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//
// @return:
//   - результат 1 (*ConferenceRepository): созданный компонент с переданными зависимостями.
func NewConferenceRepository(db *gorm.DB) *ConferenceRepository { return &ConferenceRepository{db: db} }

// Create создаёт новое состояние конференций и членств участников по переданным параметрам.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conference (conferences.Conference): конференция либо её идентификатор, ограничивающий область операции.
//   - owner (conferences.Participant): значение owner типа conferences.Participant, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (conferences.Conference): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRepository) Create(ctx context.Context, conference conferences.Conference, owner conferences.Participant) (conferences.Conference, error) {
	err := r.db.WithContext(ctx).Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			if err := tx.Create(&conference).Error; err != nil {
				return err
			}
			return tx.Create(&owner).Error
		})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "conferences_invite_code_key" {
		return conferences.Conference{}, conferences.ErrInviteCollision
	}
	return conference, err
}

// Get читает состояние конференций и членств участников для дальнейшей обработки или ответа.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (conferences.Conference): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRepository) Get(ctx context.Context, id string) (conferences.Conference, error) {
	return findConference(r.db.WithContext(ctx), id, false)
}

// GetByInvite находит конференцию по действующему коду приглашения.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - code (string): код приглашения или машинный код результата.
//
// @return:
//   - результат 1 (conferences.Conference): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRepository) GetByInvite(ctx context.Context, code string) (conferences.Conference, error) {
	var conference conferences.Conference
	err := r.db.WithContext(ctx).Where("invite_code = ?", code).Take(&conference).Error
	return conference, mapNotFound(err)
}

// ListForUser возвращает конференции, доступные указанному пользователю.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - limit (int): максимальное число элементов страницы или порции обработки.
//   - offset (int): число элементов, пропускаемых перед началом страницы.
//
// @return:
//   - результат 1 ([]conferences.Conference): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRepository) ListForUser(ctx context.Context, userID string, limit, offset int) ([]conferences.Conference, error) {
	items := []conferences.Conference{}
	err := r.db.WithContext(ctx).Model(&conferences.Conference{}).
		Joins("JOIN conference_participants p ON p.conference_id = conferences.id").
		Where("p.user_id = ? AND p.admission_state IN ('admitted','waiting')", userID).
		Where("NOT EXISTS (SELECT 1 FROM conference_chat_preferences cp WHERE cp.conference_id=conferences.id AND cp.user_id=? AND cp.left_at IS NOT NULL)", userID).
		Order("conferences.created_at DESC, conferences.id").
		Limit(limit).Offset(offset).Find(&items).Error
	return items, err
}

// Membership читает членство пользователя в заданной конференции.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//
// @return:
//   - результат 1 (conferences.Participant): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRepository) Membership(ctx context.Context, id, userID string) (conferences.Participant, error) {
	return findMembership(r.db.WithContext(ctx), id, userID)
}

// Participants возвращает разрешённую страницу участников конференции.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - limit (int): максимальное число элементов страницы или порции обработки.
//   - offset (int): число элементов, пропускаемых перед началом страницы.
//
// @return:
//   - результат 1 ([]conferences.Participant): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRepository) Participants(ctx context.Context, id string, limit, offset int) ([]conferences.Participant, error) {
	items := []conferences.Participant{}
	err := r.db.WithContext(ctx).Where("conference_id = ?", id).
		Order("created_at ASC, id").Limit(limit).Offset(offset).Find(&items).Error
	return items, err
}

// Transition выполняет разрешённый переход состояния конференции или записи.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - target (conferences.Status): целевой объект, участник или состояние операции.
//
// @return:
//   - результат 1 (conferences.Conference): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRepository) Transition(ctx context.Context, id, userID string, target conferences.Status) (conferences.Conference, error) {
	var conference conferences.Conference
	err := r.db.WithContext(ctx).Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			var err error
			conference, err = findConference(tx, id, true)
			if err != nil {
				return err
			}
			if conference.OwnerID != userID {
				return apperrors.ErrForbidden
			}
			if !conferences.CanTransition(conference.Status, target) {
				return apperrors.New(apperrors.ErrConflict, "conference status does not allow this transition")
			}
			now := time.Now().UTC()
			updates := map[string]any{"status": target}
			if target == conferences.Active {
				updates["started_at"] = now
			} else {
				updates["finished_at"] = now
			}
			if err := tx.Model(&conference).Updates(updates).Error; err != nil {
				return err
			}
			if target == conferences.Finished || target == conferences.Cancelled {
				if err := stopConferenceRecordings(tx, id); err != nil {
					return err
				}
				if err := tx.Model(&conferences.Participant{}).
					Where("conference_id = ? AND status = ?", id, conferences.Joined).
					Updates(map[string]any{"status": conferences.Left, "left_at": now, "microphone_enabled": false, "camera_enabled": false, "screen_sharing": false, "media_policy_version": gorm.Expr("media_policy_version + 1")}).Error; err != nil {
					return err
				}
			}
			conference, err = findConference(tx, id, false)
			return err
		})
	return conference, err
}

// Join создаёт или восстанавливает членство участника, учитывая приглашение, состояние встречи и зал ожидания.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - user (users.User): пользователь либо его идентификатор, определяющий область доступа.
//   - inviteCode (string): криптографически случайный код приглашения, не заменяющий авторизацию.
//
// @return:
//   - результат 1 (conferences.Participant): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRepository) Join(ctx context.Context, id string, user users.User, inviteCode string) (conferences.Participant, error) {
	var participant conferences.Participant
	err := r.db.WithContext(ctx).Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			conference, err := findConference(tx, id, true)
			if err != nil {
				return err
			}
			if conference.Status != conferences.Created && conference.Status != conferences.Active && conference.Status != conferences.Scheduled {
				return apperrors.New(apperrors.ErrConflict, "conference is closed")
			}
			participant, err = findMembership(tx, id, user.ID)
			missing := errors.Is(err, apperrors.ErrNotFound)
			if err != nil && !missing {
				return err
			}
			hasInvite := inviteCode != "" && subtle.ConstantTimeCompare([]byte(inviteCode), []byte(conference.InviteCode)) == 1
			if missing && !hasInvite {
				return apperrors.New(apperrors.ErrForbidden, "inviteCode is required for a new membership")
			}
			if !missing && participant.IsRejectedOrKicked() {
				return apperrors.New(apperrors.ErrForbidden, "this membership cannot rejoin the conference")
			}
			if !missing {
				if err := tx.Exec("UPDATE conference_chat_preferences SET left_at=NULL,updated_at=clock_timestamp() WHERE conference_id=? AND user_id=? AND left_at IS NOT NULL", id, user.ID).Error; err != nil {
					return err
				}
			}
			if !missing && participant.CanParticipate() {
				return nil
			}
			now := time.Now().UTC()
			if missing {
				userID := user.ID
				participant = conferences.Participant{ID: uuid.NewString(), ConferenceID: id, UserID: &userID,
					DisplayName: user.ParticipantName(), Role: conferences.ParticipantRole, Status: conferences.Joined, JoinedAt: &now, AdmissionState: conferences.AdmissionAdmitted}
				if conference.WaitingRoomEnabled && !hasInvite {
					participant.Status, participant.AdmissionState, participant.JoinedAt = conferences.Waiting, conferences.AdmissionWaiting, nil
				} else if conference.Status == conferences.Scheduled {
					participant.Status, participant.JoinedAt = conferences.Left, nil
				}
				return tx.Create(&participant).Error
			}
			// Ссылка даёт немедленный допуск, в том числе участнику из прежнего зала ожидания.
			// Явные отклонения и исключения проверены выше.
			if hasInvite && participant.AdmissionState == conferences.AdmissionWaiting {
				status := conferences.Joined
				var joinedAt *time.Time = &now
				if conference.Status == conferences.Scheduled {
					status, joinedAt = conferences.Left, nil
				}
				if err := tx.Model(&participant).Updates(map[string]any{"status": status, "admission_state": conferences.AdmissionAdmitted, "admission_decided_at": now, "admission_version": gorm.Expr("admission_version + 1"), "joined_at": joinedAt, "left_at": nil, "media_policy_version": gorm.Expr("media_policy_version + 1")}).Error; err != nil {
					return err
				}
				participant, err = findMembership(tx, id, user.ID)
				return err
			}
			// Без ссылки сохраняется прежнее состояние допуска.
			if participant.AdmissionState == conferences.AdmissionWaiting {
				if participant.Status == conferences.Waiting {
					return nil
				}
				return tx.Model(&participant).Update("status", conferences.Waiting).Error
			}
			if conference.Status == conferences.Scheduled {
				return nil
			}
			if err := tx.Model(&participant).Updates(map[string]any{"status": conferences.Joined, "joined_at": now, "left_at": nil, "media_policy_version": gorm.Expr("media_policy_version + 1")}).Error; err != nil {
				return err
			}
			participant, err = findMembership(tx, id, user.ID)
			return err
		})
	return participant, err
}

// Leave фиксирует выход участника, сохраняя историю членства и состояние допуска.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//
// @return:
//   - результат 1 (conferences.Participant): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRepository) Leave(ctx context.Context, id, userID string) (conferences.Participant, error) {
	var participant conferences.Participant
	err := r.db.WithContext(ctx).Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			if _, err := findConference(tx, id, true); err != nil {
				return err
			}
			var err error
			participant, err = findMembership(tx, id, userID)
			if errors.Is(err, apperrors.ErrNotFound) {
				return apperrors.ErrForbidden
			}
			if err != nil {
				return err
			}
			if participant.Status == conferences.Waiting {
				return tx.Model(&participant).Update("status", conferences.Left).Error
			}
			if participant.Status != conferences.Joined {
				return nil
			}
			if err := tx.Model(&participant).Updates(map[string]any{"status": conferences.Left, "left_at": time.Now().UTC(), "microphone_enabled": false, "camera_enabled": false, "screen_sharing": false, "media_policy_version": gorm.Expr("media_policy_version + 1")}).Error; err != nil {
				return err
			}
			participant, err = findMembership(tx, id, userID)
			return err
		})
	return participant, err
}

// findConference читает конференцию, при необходимости блокируя строку для изменения.
//
// @args
//   - db (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - locked (bool): указывает, требуется ли чтение с блокировкой строки для согласованного изменения.
//
// @return:
//   - результат 1 (conferences.Conference): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func findConference(db *gorm.DB, id string, locked bool) (conferences.Conference, error) {
	if locked {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var conference conferences.Conference
	err := db.Where("id = ?", id).Take(&conference).Error
	return conference, mapNotFound(err)
}

// findMembership читает членство пользователя внутри конкретной конференции.
//
// @args
//   - db (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//
// @return:
//   - результат 1 (conferences.Participant): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func findMembership(db *gorm.DB, id, userID string) (conferences.Participant, error) {
	var participant conferences.Participant
	err := db.Where("conference_id = ? AND user_id = ?", id, userID).Take(&participant).Error
	return participant, mapNotFound(err)
}

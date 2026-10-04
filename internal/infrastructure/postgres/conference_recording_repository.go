package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ConferenceRecordingRepository реализует постоянное хранение конференций и членств участников через GORM.
//   - db: подключение или текущая транзакция GORM, задающая контекст доступа к базе.
type ConferenceRecordingRepository struct{ db *gorm.DB }

// NewConferenceRecordingRepository создаёт и связывает зависимости компонента ConferenceRecordingRepository, используемого в жизненном цикле конференций и правах участников.
//
// @args
//   - db (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//
// @return:
//   - результат 1 (*ConferenceRecordingRepository): созданный компонент с переданными зависимостями.
func NewConferenceRecordingRepository(db *gorm.DB) *ConferenceRecordingRepository {
	return &ConferenceRecordingRepository{db: db}
}

type RecordingOutbox = records.OutboxCommand

// Start запускает обработку конференций и членств участников и подготавливает связанные ресурсы.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - segmentDuration (int): значение segmentDuration типа int, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (records.Record): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (bool): признак выполнения проверяемого условия или изменения состояния.
//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRecordingRepository) Start(ctx context.Context, userID, conferenceID string, segmentDuration int) (records.Record, bool, error) {
	return r.StartMode(ctx, userID, conferenceID, segmentDuration, records.ModeComposite)
}

// StartMode создаёт запись выбранной стратегии, сериализуя её с модерацией и завершением встречи.
// Только допущенный владелец может начать запись. Любая незавершённая запись встречи,
// включая подготовку и сохранение файлов, запрещает повторный старт во всех режимах.
// @args ctx — срок выполнения; userID/conferenceID — актор и встреча; segmentDuration — секунды фрагмента; mode — серверная стратегия.
// @return запись, признак создания и ошибка прав/состояния.
func (r *ConferenceRecordingRepository) StartMode(ctx context.Context, userID, conferenceID string, segmentDuration int, mode string) (records.Record, bool, error) {
	if !records.ValidConferenceMode(mode) {
		return records.Record{}, false, apperrors.ErrInvalidInput
	}
	var record records.Record
	created := false
	err := r.db.WithContext(ctx).Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			conference, err := findConference(tx, conferenceID, true)
			if err != nil {
				return err
			}
			actor, err := findMembership(tx, conferenceID, userID)
			if err != nil {
				return membershipError(err)
			}
			if conference.OwnerID != userID || actor.Role != conferences.Owner || !actor.CanParticipate() {
				return apperrors.ErrForbidden
			}
			if conference.Status != conferences.Active {
				return apperrors.New(apperrors.ErrConflict, "conference is not active")
			}
			err = tx.Where("platform_conference_id = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus') AND status IN ?", conferenceID, activeRecordingStatuses()).Take(&record).Error
			if err == nil {
				return apperrors.New(apperrors.ErrConflict, "recording is already active")
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			record = records.Record{UUID: uuid.NewString(), ConferenceID: conferenceID, PlatformConferenceID: &conferenceID, RequestedBy: &userID, Mode: mode, SourceType: "conference", TransportType: "sfu", Status: records.StatusStarting, QualityMode: "auto", SegmentDurationSec: segmentDuration, NeedPreview: mode != records.ModeAudioOnly && mode != records.ModeIndividualTracks}
			if err = tx.Create(&record).Error; err != nil {
				return err
			}
			if err = enqueueRecording(tx, record.UUID, "record.start", ""); err != nil {
				return err
			}
			created = true
			return nil
		})
	return record, created, err
}

// Stop останавливает активную обработку конференций и членств участников и освобождает связанные ресурсы.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - recordID (string): внешний UUID задачи записи.
//
// @return:
//   - результат 1 (records.Record): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRecordingRepository) Stop(ctx context.Context, userID, conferenceID, recordID string) (records.Record, error) {
	var record records.Record
	err := r.db.WithContext(ctx).Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			conference, err := findConference(tx, conferenceID, true)
			if err != nil {
				return err
			}
			actor, err := findMembership(tx, conferenceID, userID)
			if err != nil {
				return membershipError(err)
			}
			if conference.OwnerID != userID || actor.Role != conferences.Owner || !actor.CanReadHistory() {
				return apperrors.ErrForbidden
			}
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ? AND platform_conference_id = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus')", recordID, conferenceID).Take(&record).Error; err != nil {
				return mapNotFound(err)
			}
			if records.IsTerminalStatus(record.Status) || record.Status == records.StatusFinalizing || record.Status == records.StatusUploading {
				return nil
			}
			if record.Status != records.StatusStopping {
				if err = tx.Model(&record).Updates(map[string]any{"status": records.StatusStopping, "stopped_at": time.Now().UTC(), "ended_reason": "owner_requested"}).Error; err != nil {
					return err
				}
			}
			return enqueueRecording(tx, record.UUID, "record.stop", "owner_requested")
		})
	return record, err
}

// Accessible проверяет связь записи с конференцией и право пользователя читать её.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - recordID (string): внешний UUID задачи записи.
//
// @return:
//   - результат 1 (records.Record): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRecordingRepository) Accessible(ctx context.Context, userID, conferenceID, recordID string) (records.Record, error) {
	if err := r.authorizeRead(ctx, userID, conferenceID); err != nil {
		return records.Record{}, err
	}
	var record records.Record
	err := r.db.WithContext(ctx).Where("uuid = ? AND platform_conference_id = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus')", recordID, conferenceID).Take(&record).Error
	return record, mapNotFound(err)
}

// List возвращает ограниченный список конференций и членств участников с принятыми в данном слое фильтрами.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - limit (int): максимальное число элементов страницы или порции обработки.
//   - offset (int): число элементов, пропускаемых перед началом страницы.
//
// @return:
//   - результат 1 ([]records.Record): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRecordingRepository) List(ctx context.Context, userID, conferenceID string, limit, offset int) ([]records.Record, error) {
	if err := r.authorizeRead(ctx, userID, conferenceID); err != nil {
		return nil, err
	}
	items := []records.Record{}
	err := r.db.WithContext(ctx).Where("platform_conference_id = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus')", conferenceID).Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&items).Error
	return items, err
}

// authorizeRead проверяет допуск к записи конференции, не выдавая доступ по одному идентификатору.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRecordingRepository) authorizeRead(ctx context.Context, userID, conferenceID string) error {
	participant, err := findMembership(r.db.WithContext(ctx), conferenceID, userID)
	if err != nil {
		return membershipError(err)
	}
	if !participant.CanReadHistory() {
		return apperrors.ErrForbidden
	}
	return nil
}

// activeRecordingStatuses возвращает состояния незавершённых записей для выборки и остановки конференции.
//
// @return:
//   - результат 1 ([]string): собранные элементы результата; состав ограничивается параметрами операции.
func activeRecordingStatuses() []string {
	return []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}
}

// enqueueRecording сохраняет команду записи в транзакционном журнале доставки вместе с изменением записи.
//
// @args
//   - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//   - recordID (string): внешний UUID задачи записи.
//   - command (string): внутренняя команда с типом операции и серверной идентичностью ресурса.
//   - reason (string): причина завершения, отказа или изменения состояния.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func enqueueRecording(tx *gorm.DB, recordID, command, reason string) error {
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&RecordingOutbox{RecordID: recordID, CommandType: command, Reason: reason}).Error
}

// stopConferenceRecordings переводит активные записи закрываемой встречи в остановку и сохраняет команды завершения.
//
// @args
//   - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func stopConferenceRecordings(tx *gorm.DB, conferenceID string) error {
	var rows []records.Record
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("platform_conference_id = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus') AND status IN ?", conferenceID, []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded, records.StatusStopping}).Find(&rows).Error; err != nil {
		return err
	}
	for _, record := range rows {
		if record.Status != records.StatusStopping {
			if err := tx.Model(&record).Updates(map[string]any{"status": records.StatusStopping, "stopped_at": time.Now().UTC(), "ended_reason": "conference_finished"}).Error; err != nil {
				return err
			}
		}
		if err := enqueueRecording(tx, record.UUID, "record.stop", "conference_finished"); err != nil {
			return err
		}
	}
	return nil
}

// ClaimCommand захватывает очередную команду записи с ограниченным сроком обработки и соблюдением порядка.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//
// @return:
//   - результат 1 (RecordingOutbox): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (records.Record): значение, подготовленное операцией для вызывающей стороны.
//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRecordingRepository) ClaimCommand(ctx context.Context) (RecordingOutbox, records.Record, error) {
	var out RecordingOutbox
	var record records.Record
	err := r.db.WithContext(ctx).Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			err := tx.Raw(`SELECT o.* FROM recording_outbox o WHERE o.published_at IS NULL
		AND (o.claimed_until IS NULL OR o.claimed_until < now())
		AND NOT EXISTS (SELECT 1 FROM recording_outbox prev WHERE prev.record_id=o.record_id AND prev.id<o.id AND prev.published_at IS NULL)
		ORDER BY o.id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&out).Error
			if err != nil {
				return err
			}
			if out.ID == 0 {
				return apperrors.ErrNotFound
			}
			token := uuid.NewString()
			until := time.Now().UTC().Add(15 * time.Second)
			out.ClaimToken = &token
			out.ClaimedUntil = &until
			if err = tx.Model(&out).Updates(map[string]any{"claim_token": token, "claimed_until": until}).Error; err != nil {
				return err
			}
			return tx.Where("uuid = ?", out.RecordID).Take(&record).Error
		})
	return out, record, err
}

// CompleteCommand подтверждает обработку команды только для её действующего владельца.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - command (RecordingOutbox): внутренняя команда с типом операции и серверной идентичностью ресурса.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRecordingRepository) CompleteCommand(ctx context.Context, command RecordingOutbox) error {
	return r.db.WithContext(ctx).Model(&RecordingOutbox{}).Where("id = ? AND claim_token = ?", command.ID, command.ClaimToken).Updates(map[string]any{"published_at": time.Now().UTC(), "claimed_until": nil}).Error
}

// RetryCommand назначает повторную попытку доставки команды после временной ошибки.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - command (RecordingOutbox): внутренняя команда с типом операции и серверной идентичностью ресурса.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ConferenceRecordingRepository) RetryCommand(ctx context.Context, command RecordingOutbox) error {
	return r.db.WithContext(ctx).Model(&RecordingOutbox{}).Where("id = ? AND claim_token = ?", command.ID, command.ClaimToken).Updates(map[string]any{"claimed_until": time.Now().UTC().Add(time.Second)}).Error
}

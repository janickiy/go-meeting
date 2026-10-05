package integration_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	notifications "github.com/janickiy/go-recorder/internal/domain/notifications"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"gorm.io/gorm"
)

// TestStageFiveNotificationJobsCaptureEveryDecisionAndDeduplicate проверяет фиксацию каждого решения и дедупликацию уведомлений.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFiveNotificationJobsCaptureEveryDecisionAndDeduplicate(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewConferenceRepository(f.db)
	c, err := f.service.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "Admission jobs", WaitingRoomEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Join(ctx, c.ID, f.owner, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Transition(ctx, c.ID, f.owner.ID, conferences.Active); err != nil {
		t.Fatal(err)
	}
	p, err := repo.Join(ctx, c.ID, f.member, c.InviteCode)
	if err != nil {
		t.Fatal(err)
	}
	seedLegacyWaitingMembership(t, f.db, p.ID)
	var count int64
	if err = f.db.Table("notification_jobs").Where("entity_id=?", p.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("initial waiting generated a decision", count, err)
	}
	if _, err = repo.DecideAdmission(ctx, c.ID, f.owner.ID, p.ID, conferences.AdmissionRequest{Decision: "admit"}); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Moderate(ctx, c.ID, f.owner.ID, p.ID, conferences.ModerationRequest{Action: "kick"}); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Moderate(ctx, c.ID, f.owner.ID, p.ID, conferences.ModerationRequest{Action: "kick"}); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Table("notification_jobs").Where("entity_id=?", p.ID).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("rapid decisions lost/duplicated", count, err)
	}
	rollback := errors.New("rollback test mutation")
	err = f.db.Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			if err := tx.Exec("UPDATE conference_participants SET admission_version=admission_version+1,admission_decided_at=now() WHERE id=?", p.ID).Error; err != nil {
				return err
			}
			return rollback
		})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if err = f.db.Table("notification_jobs").Where("entity_id=?", p.ID).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("rolled back decision produced job", count, err)
	}
	record, _, err := pg.NewConferenceRecordingRepository(f.db).Start(ctx, f.owner.ID, f.conference.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err = f.db.Model(&records.Record{}).Where("id=?", record.ID).Update("status", records.StatusReady).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = f.db.Table("notification_jobs").Where("entity_id=?", record.UUID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("duplicate ready jobs", count, err)
	}
	// Тысячи завершённых исторических событий не должны сканироваться на каждом периодическом запуске.
	if err = f.db.Exec(`INSERT INTO notification_jobs(kind,entity_id,entity_version,conference_id,user_id,participant_id,admission_state,processed_at)
        SELECT 'admission.decided',?::uuid,n,?::uuid,?::uuid,?::uuid,'kicked',now() FROM generate_series(1000,3999) n`, p.ID, c.ID, f.member.ID, p.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.db.Exec("ANALYZE notification_jobs").Error; err != nil {
		t.Fatal(err)
	}
	var plan []struct {
		QueryPlan string `gorm:"column:QUERY PLAN"`
	}
	if err = f.db.Raw("EXPLAIN (ANALYZE, BUFFERS) SELECT id FROM notification_jobs WHERE processed_at IS NULL ORDER BY available_at,id LIMIT 100 FOR UPDATE SKIP LOCKED").Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}
	indexed := false
	for _, row := range plan {
		t.Log(row.QueryPlan)
		indexed = indexed || strings.Contains(row.QueryPlan, "idx_notification_jobs_pending")
	}
	if !indexed {
		t.Fatal("pending event query missed its partial index")
	}
	nrepo := pg.NewNotificationRepository(f.db)
	errs := make([]error, 8)
	runConcurrent(len(errs), /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - i (int): значение i типа int, используемое согласно назначению этой операции.
		*/func(i int) { errs[i] = nrepo.Generate(ctx) })
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := nrepo.List(ctx, f.member.ID, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]int{}
	ready := 0
	for _, n := range page.Items {
		if n.Type == "admission.decided" {
			states[n.Payload.AdmissionState]++
		}
		if n.Type == "recording.ready" {
			ready++
		}
	}
	if states["admitted"] != 1 || states["kicked"] != 1 || ready != 1 {
		t.Fatalf("immutable notifications: %+v ready=%d", states, ready)
	}
	for range 3 {
		if err = nrepo.Generate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err = f.db.Table("notification_jobs").Where("processed_at IS NULL").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("pending jobs not drained", count, err)
	}
	if err = f.db.Model(&notifications.Notification{}).Count(&count).Error; err != nil || count != 4 {
		t.Fatal("notification retry duplicated rows", count, err)
	}
}

// TestStageFiveRecordingNotificationFanoutBoundedAndAtomic проверяет сценарий «этап пять запись уведомление рассылка ограниченный и Atomic», фиксируя ошибки поведения как регрессию.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFiveRecordingNotificationFanoutBoundedAndAtomic(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	nrepo := pg.NewNotificationRepository(f.db)
	us := make([]users.User, 260)
	ps := make([]conferences.Participant, len(us))
	for i := range us {
		us[i] = users.User{ID: uuid.NewString(), Email: uuid.NewString() + "@notification.example", PasswordHash: f.owner.PasswordHash}
		userID := us[i].ID
		ps[i] = conferences.Participant{ID: uuid.NewString(), ConferenceID: f.conference.ID, UserID: &userID, DisplayName: "Attendee", Role: conferences.ParticipantRole, Status: conferences.Left, AdmissionState: conferences.AdmissionAdmitted}
	}
	if err := f.db.CreateInBatches(us, 100).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.CreateInBatches(ps, 100).Error; err != nil {
		t.Fatal(err)
	}
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	// @args
	//   - waiting (bool): логический признак waiting, управляющий соответствующей веткой обработки.
	//
	// @return:
	//   - результат 1 (conferences.Participant): значение, подготовленное операцией для вызывающей стороны.
	createAttendee := func(waiting bool) conferences.Participant {
		u := users.User{ID: uuid.NewString(), Email: uuid.NewString() + "@notification.example", PasswordHash: f.owner.PasswordHash}
		if err := f.db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		p := conferences.Participant{ID: uuid.NewString(), ConferenceID: f.conference.ID, UserID: &u.ID, DisplayName: "Late attendee", Role: conferences.ParticipantRole, Status: conferences.Left, AdmissionState: conferences.AdmissionAdmitted}
		if waiting {
			p.Status, p.AdmissionState = conferences.Waiting, conferences.AdmissionWaiting
		}
		if err := f.db.Create(&p).Error; err != nil {
			t.Fatal(err)
		}
		return p
	}
	lateAdmitted := createAttendee(true)
	record, _, err := pg.NewConferenceRecordingRepository(f.db).Start(ctx, f.owner.ID, f.conference.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Model(&records.Record{}).Where("id=?", record.ID).Update("status", records.StatusReady).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = pg.NewConferenceRepository(f.db).DecideAdmission(ctx, f.conference.ID, f.owner.ID, lateAdmitted.ID, conferences.AdmissionRequest{Decision: "admit"}); err != nil {
		t.Fatal(err)
	}
	lateCreated := createAttendee(false)
	rollback := errors.New("rollback generation")
	err = f.db.Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			if err := pg.NewNotificationRepository(tx).Generate(ctx); err != nil {
				return err
			}
			return rollback
		})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	var count int64
	if err = f.db.Model(&notifications.Notification{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("notifications escaped rollback", count, err)
	}
	var job struct {
		CursorParticipantID *string
		ProcessedAt         *time.Time
	}
	if err = f.db.Table("notification_jobs").Where("entity_id=?", record.UUID).Take(&job).Error; err != nil || job.CursorParticipantID != nil || job.ProcessedAt != nil {
		t.Fatal("job progress escaped rollback", job, err)
	}
	if err = nrepo.Generate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Model(&notifications.Notification{}).Where("type='recording.ready'").Count(&count).Error; err != nil || count != 100 {
		t.Fatal("fanout is not bounded to 100/job/tick", count, err)
	}
	// Каждая последующая порция заново проверяет получателей, не полагаясь на первоначальное
	// событие. Исключаем ещё не уведомлённого участника, удалённого из встречи между порциями.
	target := ps[0]
	for _, p := range ps {
		if p.ID > target.ID {
			target = p
		}
	}
	if _, err = pg.NewConferenceRepository(f.db).Moderate(ctx, f.conference.ID, f.owner.ID, target.ID, conferences.ModerationRequest{Action: "kick"}); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err = nrepo.Generate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err = f.db.Model(&notifications.Notification{}).Where("type='recording.ready'").Count(&count).Error; err != nil || count != 261 {
		t.Fatal("fanout lost/duplicated recipients", count, err)
	}
	if err = f.db.Model(&notifications.Notification{}).Where("type='recording.ready' AND user_id=?", target.UserID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("kicked participant received recording event", count, err)
	}
	if err = f.db.Model(&notifications.Notification{}).Where("type='recording.ready' AND user_id IN ?", []string{*lateAdmitted.UserID, *lateCreated.UserID}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("post-event admission/enrollment got a retrospective ready notification", count, err)
	}
	job = struct {
		CursorParticipantID *string
		ProcessedAt         *time.Time
	}{}
	if err = f.db.Table("notification_jobs").Where("entity_id=?", record.UUID).Take(&job).Error; err != nil || job.ProcessedAt == nil {
		t.Fatal("fanout never completed", err)
	}
}

// TestStageFiveSoonNotificationDedupAcrossSessionTimezones проверяет сценарий «этап пять Soon уведомление дедупликация Across сессия Timezones», фиксируя ошибки поведения как регрессию.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFiveSoonNotificationDedupAcrossSessionTimezones(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	soon := time.Now().UTC().Add(10 * time.Minute)
	c, err := f.service.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "UTC dedup", ScheduledAt: &soon})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Join(ctx, f.member.ID, c.ID, conferences.JoinRequest{InviteCode: c.InviteCode}); err != nil {
		t.Fatal(err)
	}
	for _, zone := range []string{"Pacific/Honolulu", "Europe/Moscow", "UTC"} {
		err = f.db.Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

			@args
			  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

			@return:
			  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
				if err := tx.Exec("SELECT set_config('TimeZone',?,true)", zone).Error; err != nil {
					return err
				}
				return pg.NewNotificationRepository(tx).Generate(ctx)
			})
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err = f.db.Model(&notifications.Notification{}).Where("type='conference.soon'").Count(&count).Error; err != nil || count != 2 {
		t.Fatal("timezone-dependent duplicate soon notifications", count, err)
	}
}

// TestStageFiveNotificationReadyDoesNotInvertConferenceLock проверяет сценарий «этап пять уведомление готовность выполняет не Invert конференция Lock», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFiveNotificationReadyDoesNotInvertConferenceLock(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	record, _, err := pg.NewConferenceRecordingRepository(f.db).Start(ctx, f.owner.ID, f.conference.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	ownerStop := f.db.Begin()
	if ownerStop.Error != nil {
		t.Fatal(ownerStop.Error)
	}
	defer ownerStop.Rollback()
	// Stop сначала блокирует строку конференции, а затем обращается к записи.
	if err = ownerStop.Exec("SELECT id FROM conferences WHERE id=? FOR UPDATE", f.conference.ID).Error; err != nil {
		t.Fatal(err)
	}
	readyCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	// Внешний ключ conference_id в триггере готовности ожидал бы ownerStop, удерживая
	// строку записи, что меняет порядок блокировок и создаёт взаимоблокировку со Stop.
	if err = f.db.WithContext(readyCtx).Model(&records.Record{}).Where("id=?", record.ID).Update("status", records.StatusReady).Error; err != nil {
		t.Fatalf("recording-ready trigger waited on conference lock: %v", err)
	}
	if err = ownerStop.Exec("SELECT id FROM record WHERE id=? FOR UPDATE", record.ID).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = f.db.Table("notification_jobs").Where("recording_id=?", record.UUID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("ready event not atomically persisted", count, err)
	}
}

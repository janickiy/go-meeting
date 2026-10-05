package postgres

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/domain/users"
	"gorm.io/gorm"
)

// recordingTestDatabase создаёт отдельную случайную БД только на явно указанном
// локальном тестовом сервере. Это позволяет проверять конкуренцию между реальными
// соединениями, а не вложенными транзакциями одного соединения. Основная БД не меняется.
// @args t — исполнитель теста; RECORDER_RECORDING_TEST_POSTGRES_DSN — локальный URL
// БД с префиксом recording_test_, пользователь которой может создавать тестовые БД.
// @return подключение к новой БД с актуальными миграциями; БД удаляется после теста.
func recordingTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("RECORDER_RECORDING_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set RECORDER_RECORDING_TEST_POSTGRES_DSN to an isolated local recording_test_ database")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "recording_test_") {
		t.Fatal("recording concurrency tests require an isolated local recording_test_ database URL")
	}
	control, err := Connect(dsn)
	if err != nil {
		t.Fatal(err)
	}
	controlSQL, err := control.DB()
	if err != nil {
		t.Fatal(err)
	}
	name := "recording_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = control.Exec(`CREATE DATABASE "` + name + `"`).Error; err != nil {
		_ = controlSQL.Close()
		t.Fatal(err)
	}
	var db *gorm.DB
	t.Cleanup(func() {
		if db != nil {
			connection, err := db.DB()
			if err != nil {
				t.Error(err)
			} else if err := connection.Close(); err != nil {
				t.Error(err)
			}
		}
		if err := control.Exec(`DROP DATABASE "` + name + `"`).Error; err != nil {
			t.Errorf("remove owned test database: %v", err)
		}
		_ = controlSQL.Close()
	})
	u.Path = "/" + name
	db, err = Connect(u.String())
	if err != nil {
		t.Fatal(err)
	}
	if err = RunMigrations(db, "../../../database/migrations"); err != nil {
		t.Fatal(err)
	}
	return db
}

// recordingFixture хранит независимую активную встречу с владельцем, двумя
// участниками разных ролей и пользователем без членства.
// @params db — отдельная тестовая БД; conference/owner/coHost/member/outsider — UUID сущностей.
type recordingFixture struct {
	db                                          *gorm.DB
	conference, owner, coHost, member, outsider string
}

// newRecordingFixture создаёт и фиксирует участников до запуска параллельных транзакций.
// @args t — исполнитель теста; db — изолированное подключение с миграциями.
// @return встреча и идентификаторы пользователей для проверки прав и конкуренции.
func newRecordingFixture(t *testing.T, db *gorm.DB) recordingFixture {
	t.Helper()
	f := recordingFixture{db: db, conference: uuid.NewString(), owner: uuid.NewString(), coHost: uuid.NewString(), member: uuid.NewString(), outsider: uuid.NewString()}
	now := time.Now().UTC()
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, id := range []string{f.owner, f.coHost, f.member, f.outsider} {
			if err := tx.Create(&users.User{ID: id, Email: id + "@recording.invalid", PasswordHash: "unused-test-hash"}).Error; err != nil {
				return err
			}
		}
		conference := conferences.Conference{ID: f.conference, OwnerID: f.owner, Title: "Изолированная проверка записи", InviteCode: strings.ReplaceAll(uuid.NewString(), "-", ""), Status: conferences.Active, StartedAt: &now}
		if err := tx.Create(&conference).Error; err != nil {
			return err
		}
		for _, actor := range []struct {
			id   string
			role conferences.Role
		}{{f.owner, conferences.Owner}, {f.coHost, conferences.CoHost}, {f.member, conferences.ParticipantRole}} {
			participant := conferences.Participant{ID: uuid.NewString(), ConferenceID: f.conference, UserID: &actor.id, DisplayName: "Участник теста", Role: actor.role, Status: conferences.Joined, JoinedAt: &now, AdmissionState: conferences.AdmissionAdmitted}
			if err := tx.Create(&participant).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// assertRecordingCounts проверяет, что отказ не создал вторую запись или команду.
// @args t — исполнитель теста; f — встреча; want — число записей и команд record.start.
func assertRecordingCounts(t *testing.T, f recordingFixture, want int64) {
	t.Helper()
	var count int64
	if err := f.db.Model(&records.Record{}).Where("platform_conference_id = ?", f.conference).Count(&count).Error; err != nil || count != want {
		t.Fatalf("record count=%d, want=%d: %v", count, want, err)
	}
	if err := f.db.Table("recording_outbox AS o").Joins("JOIN record AS r ON r.uuid = o.record_id").Where("r.platform_conference_id = ? AND o.command_type = 'record.start'", f.conference).Count(&count).Error; err != nil || count != want {
		t.Fatalf("start outbox count=%d, want=%d: %v", count, want, err)
	}
}

// TestConferenceRecordingActiveStatesRejectAllModes запрещает повторный старт на
// всех незавершённых стадиях, включая stopping и обработку файлов после остановки.
// @args t — исполнитель теста реального PostgreSQL.
func TestConferenceRecordingActiveStatesRejectAllModes(t *testing.T) {
	db := recordingTestDatabase(t)
	ctx := context.Background()
	for _, status := range activeRecordingStatuses() {
		t.Run(status, func(t *testing.T) {
			f := newRecordingFixture(t, db)
			repo := NewConferenceRecordingRepository(db)
			record, created, err := repo.Start(ctx, f.owner, f.conference, 5)
			if err != nil || !created {
				t.Fatalf("initial start: created=%v %v", created, err)
			}
			if err = db.Model(&record).Update("status", status).Error; err != nil {
				t.Fatal(err)
			}
			var before records.Record
			if err = db.Where("uuid = ?", record.UUID).Take(&before).Error; err != nil {
				t.Fatal(err)
			}
			for _, mode := range []string{records.ModeComposite, records.ModeAudioOnly, records.ModeIndividualTracks, records.ModeScreenFocus} {
				_, created, err = repo.StartMode(ctx, f.owner, f.conference, 7, mode)
				if !errors.Is(err, apperrors.ErrConflict) || err.Error() != "recording is already active" || created {
					t.Fatalf("repeat %s/%s: created=%v %v", status, mode, created, err)
				}
			}
			var after records.Record
			if err = db.Where("uuid = ?", record.UUID).Take(&after).Error; err != nil {
				t.Fatal(err)
			}
			if after.Status != status || after.SegmentDurationSec != 5 || !before.UpdatedAt.Equal(after.UpdatedAt) {
				t.Fatalf("conflict modified original record: %+v", after)
			}
			assertRecordingCounts(t, f, 1)
		})
	}
}

// TestConferenceRecordingTerminalStatesPermitReplacement разрешает новую запись
// после окончательного завершения предыдущей, не удаляя её историю.
// @args t — исполнитель теста реального PostgreSQL.
func TestConferenceRecordingTerminalStatesPermitReplacement(t *testing.T) {
	db := recordingTestDatabase(t)
	ctx := context.Background()
	for _, status := range []string{records.StatusReady, records.StatusPartialReady, records.StatusFailed, records.StatusCancelled} {
		t.Run(status, func(t *testing.T) {
			f := newRecordingFixture(t, db)
			repo := NewConferenceRecordingRepository(db)
			previous, _, err := repo.Start(ctx, f.owner, f.conference, 5)
			if err != nil {
				t.Fatal(err)
			}
			if err = db.Model(&previous).Update("status", status).Error; err != nil {
				t.Fatal(err)
			}
			current, created, err := repo.StartMode(ctx, f.owner, f.conference, 5, records.ModeAudioOnly)
			if err != nil || !created || current.UUID == previous.UUID {
				t.Fatalf("replacement after %s: created=%v %+v %v", status, created, current, err)
			}
			assertRecordingCounts(t, f, 2)
		})
	}
}

// TestConferenceRecordingConcurrentStarts проверяет блокировку между отдельными
// экземплярами репозитория, разными режимами и пользователями: ровно один UUID и outbox.
// @args t — исполнитель теста конкурентных PostgreSQL-транзакций.
func TestConferenceRecordingConcurrentStarts(t *testing.T) {
	db := recordingTestDatabase(t)
	for _, scenario := range []string{"same_mode", "different_modes", "different_users"} {
		t.Run(scenario, func(t *testing.T) {
			f := newRecordingFixture(t, db)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			modes := []string{records.ModeComposite, records.ModeAudioOnly, records.ModeIndividualTracks, records.ModeScreenFocus}
			actors := []string{f.owner, f.coHost, f.member, f.outsider}
			type result struct {
				record  records.Record
				created bool
				err     error
			}
			gate, results := make(chan struct{}), make(chan result, 24)
			var wg sync.WaitGroup
			for i := range 24 {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					<-gate
					actor := f.owner
					if scenario == "different_users" {
						actor = actors[index%len(actors)]
					}
					repo := NewConferenceRecordingRepository(db)
					var r result
					if scenario == "same_mode" {
						r.record, r.created, r.err = repo.Start(ctx, actor, f.conference, 5)
					} else {
						mode := modes[index%len(modes)]
						if scenario == "different_users" {
							mode = modes[(index/len(actors))%len(modes)]
						}
						r.record, r.created, r.err = repo.StartMode(ctx, actor, f.conference, 5, mode)
					}
					results <- r
				}(i)
			}
			close(gate)
			wg.Wait()
			close(results)
			successes, conflicts, forbidden := 0, 0, 0
			id := ""
			for r := range results {
				switch {
				case r.err == nil:
					successes++
					id = r.record.UUID
					if !r.created || id == "" || r.record.RequestedBy == nil || (*r.record.RequestedBy != f.owner && *r.record.RequestedBy != f.coHost && *r.record.RequestedBy != f.member) {
						t.Fatalf("invalid accepted record: %+v", r)
					}
				case errors.Is(r.err, apperrors.ErrConflict):
					conflicts++
					if r.created || r.err.Error() != "recording is already active" {
						t.Fatalf("invalid conflict: %+v", r)
					}
				case errors.Is(r.err, apperrors.ErrForbidden):
					forbidden++
					if r.created || r.record.UUID != "" {
						t.Fatalf("forbidden start disclosed or created record: %+v", r)
					}
				default:
					t.Fatalf("unexpected concurrent error: %v", r.err)
				}
			}
			wantConflicts, wantForbidden := 23, 0
			if scenario == "different_users" {
				wantConflicts, wantForbidden = 17, 6
			}
			if successes != 1 || conflicts != wantConflicts || forbidden != wantForbidden {
				t.Fatalf("successes=%d conflicts=%d forbidden=%d", successes, conflicts, forbidden)
			}
			var saved records.Record
			if err := db.Where("platform_conference_id = ?", f.conference).Take(&saved).Error; err != nil || saved.UUID != id {
				t.Fatalf("accepted UUID differs from saved record: %+v %v", saved, err)
			}
			assertRecordingCounts(t, f, 1)
		})
	}
}

// Проверяет право запуска всех ролей аккаунта, запрет гостю и недопущенным,
// а также остановку инициатором/организатором без разрешения чужим участникам.
func TestConferenceRecordingAccountPermissions(t *testing.T) {
	db := recordingTestDatabase(t)
	ctx := context.Background()
	for _, role := range []string{"owner", "co_host", "participant"} {
		t.Run(role, func(t *testing.T) {
			f := newRecordingFixture(t, db)
			actor := map[string]string{"owner": f.owner, "co_host": f.coHost, "participant": f.member}[role]
			repo := NewConferenceRecordingRepository(db)
			record, created, err := repo.Start(ctx, actor, f.conference, 5)
			if err != nil || !created || record.RequestedBy == nil || *record.RequestedBy != actor {
				t.Fatalf("account start: created=%v %+v %v", created, record, err)
			}
			other := f.coHost
			if actor == other {
				other = f.member
			}
			if _, err := repo.Stop(ctx, other, f.conference, record.UUID); !errors.Is(err, apperrors.ErrForbidden) {
				t.Fatalf("unrelated participant stopped recording: %v", err)
			}
			stopped, err := repo.Stop(ctx, actor, f.conference, record.UUID)
			if err != nil || stopped.Status != records.StatusStopping {
				t.Fatalf("initiator stop: %+v %v", stopped, err)
			}
			if _, err := repo.Stop(ctx, f.owner, f.conference, record.UUID); err != nil {
				t.Fatalf("owner stop: %v", err)
			}
			assertRecordingCounts(t, f, 1)
		})
	}
	for _, active := range []bool{false, true} {
		f := newRecordingFixture(t, db)
		repo := NewConferenceRecordingRepository(db)
		want := int64(0)
		var current records.Record
		if active {
			var err error
			current, _, err = repo.Start(ctx, f.owner, f.conference, 5)
			if err != nil {
				t.Fatal(err)
			}
			want = 1
		}
		// Синтетический гость имеет настоящее joined membership и JWT, но не аккаунт.
		if err := db.Model(&users.User{}).Where("id = ?", f.member).Update("guest_conference_id", f.conference).Error; err != nil {
			t.Fatal(err)
		}
		for _, user := range []string{f.member, f.outsider} {
			record, created, err := repo.Start(ctx, user, f.conference, 5)
			if !errors.Is(err, apperrors.ErrForbidden) || created || record.UUID != "" {
				t.Fatalf("unauthorized start: %+v %v", record, err)
			}
			if active {
				// Даже подставленный requested_by не даёт гостю прав остановки.
				if err := db.Model(&current).Update("requested_by", user).Error; err != nil {
					t.Fatal(err)
				}
				if _, err := repo.Stop(ctx, user, f.conference, current.UUID); !errors.Is(err, apperrors.ErrForbidden) {
					t.Fatalf("unauthorized stop: %v", err)
				}
			}
		}
		for _, state := range []struct {
			status    conferences.ParticipantStatus
			admission conferences.AdmissionState
		}{
			{conferences.Waiting, conferences.AdmissionWaiting},
			{conferences.Rejected, conferences.AdmissionRejected},
			{conferences.Kicked, conferences.AdmissionKicked},
			{conferences.Left, conferences.AdmissionAdmitted},
		} {
			if err := db.Model(&conferences.Participant{}).Where("conference_id = ? AND user_id = ?", f.conference, f.coHost).Updates(map[string]any{"status": state.status, "admission_state": state.admission}).Error; err != nil {
				t.Fatal(err)
			}
			record, created, err := repo.Start(ctx, f.coHost, f.conference, 5)
			if !errors.Is(err, apperrors.ErrForbidden) || created || record.UUID != "" {
				t.Fatalf("unadmitted start: %+v %v", record, err)
			}
		}
		assertRecordingCounts(t, f, want)
	}
}

// TestConferenceRecordingUniqueIndexRejectsBypass проверяет последнюю защиту БД:
// даже прямое создание записи в обход приложения не допускает два активных режима.
// @args t — исполнитель теста частичного уникального индекса.
func TestConferenceRecordingUniqueIndexRejectsBypass(t *testing.T) {
	db := recordingTestDatabase(t)
	f := newRecordingFixture(t, db)
	if _, _, err := NewConferenceRecordingRepository(db).Start(context.Background(), f.owner, f.conference, 5); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{records.ModeComposite, records.ModeAudioOnly, records.ModeIndividualTracks, records.ModeScreenFocus} {
		duplicate := records.Record{UUID: uuid.NewString(), ConferenceID: f.conference, PlatformConferenceID: &f.conference, RequestedBy: &f.owner, Mode: mode, SourceType: "conference", TransportType: "sfu", Status: records.StatusStarting, QualityMode: "auto", SegmentDurationSec: 5}
		err := db.Create(&duplicate).Error
		var constraint *pgconn.PgError
		if !errors.As(err, &constraint) || constraint.Code != "23505" || (constraint.ConstraintName != "idx_record_one_active_composite" && constraint.ConstraintName != "idx_record_one_active_conference") {
			t.Fatalf("missing active-record constraint for %s: %v", mode, err)
		}
	}
	assertRecordingCounts(t, f, 1)
}

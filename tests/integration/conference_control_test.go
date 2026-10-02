package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	conferencesapp "github.com/janickiy/go-recorder/internal/app/conferences"
	recordingsapp "github.com/janickiy/go-recorder/internal/app/recordings"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	conferenceusecase "github.com/janickiy/go-recorder/internal/usecase/conferences"
	realtimeusecase "github.com/janickiy/go-recorder/internal/usecase/realtime"
	"github.com/janickiy/go-recorder/internal/usecase/recorder"
	"github.com/janickiy/go-recorder/internal/usecase/recordings"
)

// policyProbe хранит изолированное состояние тестового компонента «политика Probe».
// @params:
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - last: значение last типа media.ParticipantPolicy, используемое согласно назначению этой операции.
//   - fail: логический признак fail, управляющий соответствующей веткой обработки.
type policyProbe struct {
	mu   sync.Mutex
	last media.ParticipantPolicy
	fail bool
}

// SetParticipantPolicy передаёт актуальную политику участника владельцу медиа-комнаты.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - _ (context.Context): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - _ (string): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - _ (string): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - policy (media.ParticipantPolicy): актуальные ограничения медиа и версия модерации участника.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (p *policyProbe) SetParticipantPolicy(_ context.Context, _, _ string, policy media.ParticipantPolicy) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.last = policy
	if p.fail {
		return errors.New("worker offline")
	}
	return nil
}

// TestStageFourControlPermissionsAndRecordingTransactions проверяет сценарий «этап четыре управление полномочия и запись Transactions», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFourControlPermissionsAndRecordingTransactions(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	conferenceRepo := pg.NewConferenceRepository(f.db)
	recordRepo := pg.NewRecordRepository(f.db)
	recordingRepo := pg.NewConferenceRecordingRepository(f.db)
	probe := &policyProbe{}
	control := conferenceusecase.NewControlService(conferenceRepo, probe, f.hubs[0])
	recordingService := recordings.NewConferenceService(recordingRepo, recorder.NewService(recordRepo, nil, nil, nil), nil, nil, f.hubs[0])
	router := gin.New()
	auth := httpmiddleware.Authenticate(f.tokens)
	httptransport.RegisterControlRoutes(router, conferencesapp.NewControlHandler(control), auth)
	httptransport.RegisterConferenceRecordingRoutes(router, recordingsapp.NewHandler(recordingService), auth)
	api := stageOneAPI{router: router}
	path := "/conferences/" + f.conference.ID
	member, err := conferenceRepo.Membership(ctx, f.conference.ID, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := conferenceRepo.Membership(ctx, f.conference.ID, f.owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	memberSocket := f.connect(t, 0, f.memberToken, f.conference.ID)
	sequence := 0
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	// @args
	//   - state (map[string]any): индекс значений state для поиска и согласования состояния.
	//
	// @return:
	//   - результат 1 (map[string]any): значение, подготовленное операцией для вызывающей стороны.
	mediaUpdate := func(state map[string]any) map[string]any {
		sequence++
		state["connectionId"] = memberSocket.state.ConnectionID
		state["sequence"] = sequence
		return state
	}
	api.expect(t, "PUT", path+"/participants/me/media", f.memberToken, mediaUpdate(map[string]any{"microphoneEnabled": true, "cameraEnabled": true, "screenSharing": false}), 200, nil)
	api.expect(t, "POST", path+"/participants/"+member.ID+"/moderation", f.memberToken, map[string]any{"action": "mute", "blocked": true}, 403, nil)
	var updated struct{ Item conferences.ParticipantView }
	api.expect(t, "POST", path+"/participants/"+member.ID+"/moderation", f.ownerToken, map[string]any{"action": "mute", "blocked": true}, 200, &updated)
	firstVersion := updated.Item.MediaPolicyVersion
	if !updated.Item.MicrophoneBlocked || updated.Item.MicrophoneEnabled {
		t.Fatal("mute did not persist policy and state")
	}
	api.expect(t, "POST", path+"/participants/"+member.ID+"/moderation", f.ownerToken, map[string]any{"action": "mute", "blocked": true}, 200, &updated)
	if updated.Item.MediaPolicyVersion != firstVersion {
		t.Fatal("duplicate mute changed policy version")
	}
	api.expect(t, "PUT", path+"/participants/me/media", f.memberToken, mediaUpdate(map[string]any{"microphoneEnabled": true}), 403, nil)
	api.expect(t, "PUT", path+"/participants/me/media", f.memberToken, mediaUpdate(map[string]any{"cameraEnabled": true, "screenSharing": true}), 200, nil)
	api.expect(t, "POST", path+"/participants/"+member.ID+"/moderation", f.ownerToken, map[string]any{"action": "camera", "blocked": true}, 200, &updated)
	if !updated.Item.CameraBlocked || updated.Item.CameraEnabled || updated.Item.ScreenSharing {
		t.Fatal("video policy must clear both camera and screen state")
	}
	api.expect(t, "PUT", path+"/participants/me/media", f.memberToken, mediaUpdate(map[string]any{"screenSharing": true}), 403, nil)
	api.expect(t, "POST", path+"/participants/"+member.ID+"/moderation", f.ownerToken, map[string]any{"action": "camera", "blocked": false}, 200, &updated)
	api.expect(t, "PUT", path+"/participants/me/media", f.memberToken, mediaUpdate(map[string]any{"screenSharing": true}), 200, nil)
	api.expect(t, "POST", path+"/participants/"+member.ID+"/moderation", f.ownerToken, map[string]any{"action": "role", "role": "co_host"}, 200, &updated)
	api.expect(t, "POST", path+"/participants/"+owner.ID+"/moderation", f.memberToken, map[string]any{"action": "kick"}, 403, nil)
	api.expect(t, "POST", path+"/recordings", f.memberToken, nil, 403, nil)
	api.expect(t, "POST", path+"/recordings", "", nil, 401, nil)

	// The conference row lock + partial unique index allow one active recording
	// even when requests arrive on different API instances concurrently.
	var wg sync.WaitGroup
	ids := make(chan string, 20)
	failures := make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() {
			defer wg.Done()
			record, _, err := recordingRepo.Start(ctx, f.owner.ID, f.conference.ID, 2)
			if err != nil {
				failures <- err
				return
			}
			ids <- record.UUID
		}()
	}
	wg.Wait()
	close(ids)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	recordID := ""
	for id := range ids {
		if recordID != "" && recordID != id {
			t.Fatal("duplicate active recording")
		}
		recordID = id
	}
	if recordID == "" {
		t.Fatal("no recording created")
	}
	var count int64
	if err = f.db.Table("recording_outbox").Where("record_id = ? AND command_type = 'record.start'", recordID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("start outbox count %d: %v", count, err)
	}
	api.expect(t, "GET", path+"/recordings/"+recordID, f.memberToken, nil, 200, nil)
	api.expect(t, "POST", path+"/recordings/"+recordID+"/stop", f.memberToken, nil, 403, nil)
	legacy := recorder.NewService(recordRepo, nil, nil, nil)
	if _, err = legacy.Read(ctx, recordID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("legacy leaked recording: %v", err)
	}
	if _, err = legacy.Start(ctx, records.StartRequest{ConferenceID: f.conference.ID}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("legacy captured platform conference: %v", err)
	}

	// Stop and finish share the same conference row lock; exactly one durable
	// stop survives, and the original stop timestamp is not rewritten.
	api.expect(t, "POST", path+"/recordings/"+recordID+"/stop", f.ownerToken, nil, 202, nil)
	before, _ := recordRepo.FindByUUID(ctx, recordID)
	api.expect(t, "POST", path+"/recordings/"+recordID+"/stop", f.ownerToken, nil, 202, nil)
	after, _ := recordRepo.FindByUUID(ctx, recordID)
	if before.StoppedAt == nil || after.StoppedAt == nil || !before.StoppedAt.Equal(*after.StoppedAt) {
		t.Fatal("duplicate stop changed timestamp")
	}
	if _, err = f.service.Transition(ctx, f.owner.ID, f.conference.ID, conferences.Finished); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Table("recording_outbox").Where("record_id = ? AND command_type = 'record.stop'", recordID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("stop outbox count %d: %v", count, err)
	}
	if _, _, err = recordingRepo.Start(ctx, f.owner.ID, f.conference.ID, 2); err == nil {
		t.Fatal("recording started in closed conference")
	}
}

// TestStageFourKickSurvivesRejoinAndFailedEnforcement проверяет сценарий «этап четыре Kick Survives Rejoin и Failed Enforcement», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFourKickSurvivesRejoinAndFailedEnforcement(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewConferenceRepository(f.db)
	probe := &policyProbe{fail: true}
	control := conferenceusecase.NewControlService(repo, probe, f.hubs[0])
	member, err := repo.Membership(ctx, f.conference.ID, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	recordingRepo := pg.NewConferenceRecordingRepository(f.db)
	record, _, err := recordingRepo.Start(ctx, f.owner.ID, f.conference.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	readService := recordings.NewConferenceService(recordingRepo, recorder.NewService(pg.NewRecordRepository(f.db), nil, nil, nil), nil, nil, f.hubs[0])
	router := gin.New()
	httptransport.RegisterConferenceRecordingRoutes(router, recordingsapp.NewHandler(readService), httpmiddleware.Authenticate(f.tokens))
	api := stageOneAPI{router: router}
	path := "/conferences/" + f.conference.ID + "/recordings"
	api.expect(t, "GET", path+"/"+record.UUID, f.memberToken, nil, 200, nil)
	socket := f.connect(t, 1, f.memberToken, f.conference.ID)
	if _, err = control.Moderate(ctx, f.owner.ID, f.conference.ID, member.ID, conferences.ModerationRequest{Action: "kick"}); !errors.Is(err, apperrors.ErrUnavailable) {
		t.Fatalf("failed enforcement = %v", err)
	}
	select {
	case <-socket.done:
	case <-time.After(3 * time.Second):
		t.Fatal("kicked websocket stayed connected")
	}
	if _, err = f.service.Join(ctx, f.member.ID, f.conference.ID, conferences.JoinRequest{InviteCode: f.conference.InviteCode}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("kicked member rejoined: %v", err)
	}
	persisted, _ := repo.Membership(ctx, f.conference.ID, f.member.ID)
	if persisted.Status != conferences.Kicked || persisted.LeftAt == nil {
		t.Fatal("kick history not retained")
	}
	policy, err := repo.MediaPolicy(ctx, f.conference.ID, member.ID)
	if err != nil || !policy.Kicked {
		t.Fatalf("kick policy not durable: %+v %v", policy, err)
	}
	api.expect(t, "GET", path, f.memberToken, nil, 403, nil)
	api.expect(t, "GET", path+"/"+record.UUID, f.memberToken, nil, 403, nil)
	api.expect(t, "GET", path+"/"+record.UUID, f.ownerToken, nil, 200, nil)
}

// TestStageFourCoHostCannotChangeRolesOrCameraPolicy проверяет сценарий «этап четыре Co Host Cannot Change Roles Or Camera политика», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFourCoHostCannotChangeRolesOrCameraPolicy(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewConferenceRepository(f.db)
	member, _ := repo.Membership(ctx, f.conference.ID, f.member.ID)
	if _, err := repo.Moderate(ctx, f.conference.ID, f.owner.ID, member.ID, conferences.ModerationRequest{Action: "role", Role: conferences.CoHost}); err != nil {
		t.Fatal(err)
	}
	user := users.User{ID: uuid.NewString(), Email: "third@stage4.example", PasswordHash: f.owner.PasswordHash}
	if _, err := pg.NewUserRepository(f.db).Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	third, err := f.service.Join(ctx, user.ID, f.conference.ID, conferences.JoinRequest{InviteCode: f.conference.InviteCode})
	if err != nil {
		t.Fatal(err)
	}
	b := true
	for _, request := range []conferences.ModerationRequest{{Action: "camera", Blocked: &b}, {Action: "role", Role: conferences.CoHost}} {
		if _, err = repo.Moderate(ctx, f.conference.ID, f.member.ID, third.ID, request); !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatalf("cohost unauthorized %+v: %v", request, err)
		}
	}
	for _, request := range []conferences.ModerationRequest{{Action: "mute", Blocked: &b}, {Action: "screen", Blocked: &b}, {Action: "kick"}} {
		if _, err = repo.Moderate(ctx, f.conference.ID, f.member.ID, third.ID, request); err != nil {
			t.Fatal(err)
		}
	}
}

// TestStageFourDurableOutboxOrderingAndFencing проверяет сценарий «этап четыре Durable журнал доставки порядок и защита версии владения», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFourDurableOutboxOrderingAndFencing(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewConferenceRecordingRepository(f.db)
	record, _, err := repo.Start(ctx, f.owner.ID, f.conference.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Stop(ctx, f.owner.ID, f.conference.ID, record.UUID); err != nil {
		t.Fatal(err)
	}
	start, _, err := repo.ClaimCommand(ctx)
	if err != nil || start.CommandType != "record.start" {
		t.Fatalf("claim start %+v %v", start, err)
	}
	if _, _, err = repo.ClaimCommand(ctx); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("stop overtook unpublished start: %v", err)
	}
	if err = repo.CompleteCommand(ctx, start); err != nil {
		t.Fatal(err)
	}
	stop, _, err := repo.ClaimCommand(ctx)
	if err != nil || stop.CommandType != "record.stop" {
		t.Fatalf("claim stop %+v %v", stop, err)
	}
	// Simulate dispatcher crash after claiming but before publish. Another API
	// recovers the command; the old token must not acknowledge its new claim.
	if err = f.db.Table("recording_outbox").Where("id = ?", stop.ID).Update("claimed_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	retry, _, err := repo.ClaimCommand(ctx)
	if err != nil || retry.ID != stop.ID || *retry.ClaimToken == *stop.ClaimToken {
		t.Fatalf("claim recovery %+v %v", retry, err)
	}
	if err = repo.CompleteCommand(ctx, stop); err != nil {
		t.Fatal(err)
	}
	var persisted records.OutboxCommand
	if err = f.db.Where("id = ?", retry.ID).Take(&persisted).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.PublishedAt != nil {
		t.Fatal("expired dispatcher acknowledged successor's command")
	}
	if err = repo.CompleteCommand(ctx, retry); err != nil {
		t.Fatal(err)
	}
}

// TestStageFourStartVersusFinish проверяет сценарий «этап четыре запуск Versus Finish», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFourStartVersusFinish(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewConferenceRecordingRepository(f.db)
	for range 8 {
		conference, err := f.service.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "start versus finish"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.service.Join(ctx, f.owner.ID, conference.ID, conferences.JoinRequest{}); err != nil {
			t.Fatal(err)
		}
		if _, err = f.service.Transition(ctx, f.owner.ID, conference.ID, conferences.Active); err != nil {
			t.Fatal(err)
		}
		gate := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var startErr, finishErr error
		go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() { defer wg.Done(); <-gate; _, _, startErr = repo.Start(ctx, f.owner.ID, conference.ID, 2) }()
		go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() {
			defer wg.Done()
			<-gate
			_, finishErr = f.service.Transition(ctx, f.owner.ID, conference.ID, conferences.Finished)
		}()
		close(gate)
		wg.Wait()
		if finishErr != nil {
			t.Fatal(finishErr)
		}
		if startErr != nil && !errors.Is(startErr, apperrors.ErrConflict) && !errors.Is(startErr, apperrors.ErrForbidden) {
			t.Fatal(startErr)
		}
		var rows []records.Record
		if err = f.db.Where("platform_conference_id = ?", conference.ID).Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if len(rows) > 1 {
			t.Fatal("duplicate recording after finish race")
		}
		if len(rows) == 1 {
			if rows[0].Status != records.StatusStopping {
				t.Fatalf("finished conference recording left %s", rows[0].Status)
			}
			var count int64
			f.db.Table("recording_outbox").Where("record_id = ? AND command_type = 'record.stop'", rows[0].UUID).Count(&count)
			if count != 1 {
				t.Fatalf("automatic stop missing: %d", count)
			}
		}
	}
}

// TestStageFourLastPhysicalDisconnectClearsMediaState проверяет сценарий «этап четыре последний Physical отключение Clears медиа состояние», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFourLastPhysicalDisconnectClearsMediaState(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewConferenceRepository(f.db)
	control := conferenceusecase.NewControlService(repo, &policyProbe{}, f.hubs[0])
	for _, hub := range f.hubs {
		hub.SetDisconnectObserver(realtimeusecase.DisconnectObservers{control})
	}
	owner := f.connect(t, 0, f.ownerToken, f.conference.ID)
	first := f.connect(t, 0, f.memberToken, f.conference.ID)
	second := f.connect(t, 1, f.memberToken, f.conference.ID)
	member, err := control.UpdateMediaState(ctx, f.member.ID, f.conference.ID, conferences.MediaState{ConnectionID: second.state.ConnectionID, Sequence: 1, MicrophoneEnabled: true, CameraEnabled: true, ScreenSharing: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = first.conn.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var active int64
		if err = f.db.Table("participant_sessions").Where("participant_id = ? AND status = 'connected'", member.ID).Count(&active).Error; err != nil {
			t.Fatal(err)
		}
		if active == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first physical session did not close")
		}
		time.Sleep(10 * time.Millisecond)
	}
	state, err := repo.Membership(ctx, f.conference.ID, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !state.MicrophoneEnabled || !state.CameraEnabled || !state.ScreenSharing {
		t.Fatal("first tab disconnect cleared the second tab's state")
	}
	_ = second.conn.Close()
	owner.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - event (realtime.Envelope): конверт входящего или публикуемого события.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(event realtime.Envelope) bool {
			if event.Type != "participant.media.updated" {
				return false
			}
			var payload struct{ Participant conferences.ParticipantView }
			if json.Unmarshal(event.Data, &payload) != nil {
				return false
			}
			return payload.Participant.ID == member.ID && !payload.Participant.MicrophoneEnabled && !payload.Participant.CameraEnabled && !payload.Participant.ScreenSharing
		})
	state, err = repo.Membership(ctx, f.conference.ID, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.MicrophoneEnabled || state.CameraEnabled || state.ScreenSharing {
		t.Fatal("last disconnected participant retained media flags")
	}
	first = f.connect(t, 0, f.memberToken, f.conference.ID)
	second = f.connect(t, 1, f.memberToken, f.conference.ID)
	if _, err = control.UpdateMediaState(ctx, f.member.ID, f.conference.ID, conferences.MediaState{ConnectionID: first.state.ConnectionID, Sequence: 1, MicrophoneEnabled: true, CameraEnabled: true, ScreenSharing: true}); err != nil {
		t.Fatal(err)
	}
	var disconnects sync.WaitGroup
	disconnects.Add(2)
	go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

	 */func() { defer disconnects.Done(); _ = first.conn.Close() }()
	go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

	 */func() { defer disconnects.Done(); _ = second.conn.Close() }()
	disconnects.Wait()
	owner.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - event (realtime.Envelope): конверт входящего или публикуемого события.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(event realtime.Envelope) bool {
			if event.Type != "participant.media.updated" {
				return false
			}
			var payload struct{ Participant conferences.ParticipantView }
			if json.Unmarshal(event.Data, &payload) != nil {
				return false
			}
			return payload.Participant.ID == member.ID && !payload.Participant.MicrophoneEnabled && !payload.Participant.CameraEnabled && !payload.Participant.ScreenSharing
		})
}

// TestStageFourSessionMediaStateIsOrderedAndAggregated проверяет сценарий «этап четыре сессия медиа состояние является Ordered и Aggregated», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFourSessionMediaStateIsOrderedAndAggregated(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewConferenceRepository(f.db)
	control := conferenceusecase.NewControlService(repo, &policyProbe{}, f.hubs[0])
	for _, hub := range f.hubs {
		hub.SetDisconnectObserver(realtimeusecase.DisconnectObservers{control})
	}
	first := f.connect(t, 0, f.memberToken, f.conference.ID)
	second := f.connect(t, 1, f.memberToken, f.conference.ID)
	owner := f.connect(t, 0, f.ownerToken, f.conference.ID)
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	// @args
	//   - connection (string): значение connection типа string, используемое согласно назначению этой операции.
	//   - sequence (int64): серверный монотонный номер сообщения или команды.
	//   - microphone (bool): логический признак microphone, управляющий соответствующей веткой обработки.
	//   - camera (bool): логический признак camera, управляющий соответствующей веткой обработки.
	//   - screen (bool): логический признак screen, управляющий соответствующей веткой обработки.
	//
	// @return:
	//   - результат 1 (conferences.Participant): значение, подготовленное операцией для вызывающей стороны.
	update := func(connection string, sequence int64, microphone, camera, screen bool) conferences.Participant {
		t.Helper()
		p, err := repo.UpdateMediaState(ctx, f.conference.ID, f.member.ID, conferences.MediaState{ConnectionID: connection, Sequence: sequence, MicrophoneEnabled: microphone, CameraEnabled: camera, ScreenSharing: screen})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	// @args
	//   - p (conferences.Participant): байты, переданные по контракту io.Writer.
	//   - microphone (bool): логический признак microphone, управляющий соответствующей веткой обработки.
	//   - camera (bool): логический признак camera, управляющий соответствующей веткой обработки.
	//   - screen (bool): логический признак screen, управляющий соответствующей веткой обработки.
	check := func(p conferences.Participant, microphone, camera, screen bool) {
		t.Helper()
		if p.MicrophoneEnabled != microphone || p.CameraEnabled != camera || p.ScreenSharing != screen {
			t.Fatalf("aggregate got mic=%t camera=%t screen=%t, want %t/%t/%t", p.MicrophoneEnabled, p.CameraEnabled, p.ScreenSharing, microphone, camera, screen)
		}
	}
	check(update(first.state.ConnectionID, 1, true, false, false), true, false, false)
	// An idle second tab must not clear media in the first tab.
	check(update(second.state.ConnectionID, 1, false, false, false), true, false, false)
	check(update(second.state.ConnectionID, 2, false, true, true), true, true, true)
	check(update(first.state.ConnectionID, 2, false, false, false), false, true, true)
	check(update(first.state.ConnectionID, 1, true, true, true), false, true, true)
	check(update(second.state.ConnectionID, 5, false, false, false), false, false, false)
	check(update(second.state.ConnectionID, 4, true, true, true), false, false, false)
	for _, state := range []conferences.MediaState{{Sequence: 1}, {ConnectionID: first.state.ConnectionID}, {ConnectionID: first.state.ConnectionID, Sequence: 9007199254740992}} {
		if _, err := repo.UpdateMediaState(ctx, f.conference.ID, f.member.ID, state); !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Fatalf("invalid media identity accepted: %+v %v", state, err)
		}
	}
	if _, err := repo.UpdateMediaState(ctx, f.conference.ID, f.member.ID, conferences.MediaState{ConnectionID: owner.state.ConnectionID, Sequence: 1, CameraEnabled: true}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("foreign connection accepted: %v", err)
	}
	check(update(first.state.ConnectionID, 3, true, false, false), true, false, false)
	p := update(second.state.ConnectionID, 6, false, true, true)
	b := true
	for _, action := range []string{"mute", "camera"} {
		if _, err := repo.Moderate(ctx, f.conference.ID, f.owner.ID, p.ID, conferences.ModerationRequest{Action: action, Blocked: &b}); err != nil {
			t.Fatal(err)
		}
	}
	b = false
	for _, action := range []string{"mute", "camera"} {
		if _, err := repo.Moderate(ctx, f.conference.ID, f.owner.ID, p.ID, conferences.ModerationRequest{Action: action, Blocked: &b}); err != nil {
			t.Fatal(err)
		}
	}
	// A later idle update recomputes every tab. Unblock must not resurrect the
	// previously true per-session state that moderation cleared.
	check(update(first.state.ConnectionID, 4, false, false, false), false, false, false)
	check(update(first.state.ConnectionID, 5, true, false, false), true, false, false)
	check(update(second.state.ConnectionID, 7, false, true, false), true, true, false)
	_ = first.conn.Close()
	owner.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - event (realtime.Envelope): конверт входящего или публикуемого события.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(event realtime.Envelope) bool {
			if event.Type != "participant.media.updated" {
				return false
			}
			var payload struct{ Participant conferences.ParticipantView }
			if json.Unmarshal(event.Data, &payload) != nil {
				return false
			}
			return payload.Participant.ID == p.ID && !payload.Participant.MicrophoneEnabled && payload.Participant.CameraEnabled && !payload.Participant.ScreenSharing
		})
	if _, err := repo.UpdateMediaState(ctx, f.conference.ID, f.member.ID, conferences.MediaState{ConnectionID: first.state.ConnectionID, Sequence: 6, MicrophoneEnabled: true}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("closed connection resurrected state: %v", err)
	}
}

// TestStageFourSessionMediaCloseVersusUpdate проверяет сценарий «этап четыре сессия медиа закрытие Versus обновление», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFourSessionMediaCloseVersusUpdate(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewConferenceRepository(f.db)
	sessions := pg.NewSessionRepository(f.db)
	member, err := repo.Membership(ctx, f.conference.ID, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	for range 12 {
		now := time.Now().UTC()
		session := realtime.Session{ID: uuid.NewString(), ConnectionID: uuid.NewString(), ConferenceID: f.conference.ID, ParticipantID: member.ID, UserID: f.member.ID, Status: "connected", ConnectedAt: now, LastSeenAt: now}
		if err = sessions.Open(ctx, session); err != nil {
			t.Fatal(err)
		}
		gate := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var closeErr, updateErr error
		go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() { defer wg.Done(); <-gate; closeErr = sessions.Close(ctx, session.ConnectionID, now) }()
		go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() {
			defer wg.Done()
			<-gate
			_, updateErr = repo.UpdateMediaState(ctx, f.conference.ID, f.member.ID, conferences.MediaState{ConnectionID: session.ConnectionID, Sequence: 1, MicrophoneEnabled: true})
		}()
		close(gate)
		wg.Wait()
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		if updateErr != nil && !errors.Is(updateErr, apperrors.ErrForbidden) {
			t.Fatal(updateErr)
		}
		p, _, err := repo.ClearDisconnectedMedia(ctx, f.conference.ID, member.ID)
		if err != nil {
			t.Fatal(err)
		}
		if p.MicrophoneEnabled || p.CameraEnabled || p.ScreenSharing {
			t.Fatal("close/update race retained closed-session media")
		}
	}
}

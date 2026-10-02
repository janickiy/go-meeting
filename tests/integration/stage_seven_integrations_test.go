package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	app "github.com/janickiy/go-recorder/internal/app/integrations"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	d "github.com/janickiy/go-recorder/internal/domain/integrations"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"github.com/janickiy/go-recorder/internal/infrastructure/providers"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	conferenceusecase "github.com/janickiy/go-recorder/internal/usecase/conferences"
	u "github.com/janickiy/go-recorder/internal/usecase/integrations"
	"gorm.io/gorm"
)

// integrationFixture объединяет isolated DB, generic fake gateway и сервис, не обращаясь к платным провайдерам.
type integrationFixture struct {
	db            *gorm.DB
	repo          *pg.IntegrationRepository
	jobs          *pg.JobRepository
	service       *u.Service
	conferences   *conferenceusecase.Service
	owner, member users.User
	cipher        *security.ProviderTokens
	mu            sync.Mutex
	sent          map[string]int
	fail          bool
}

// stageSevenIntegrations создаёт две учётные записи и generic HTTP fake с provider-side dedup.
// @parameters: t — контекст изолированного теста.
// @return: fixture с автоматической очисткой собственной DB/HTTP server.
func stageSevenIntegrations(t *testing.T) *integrationFixture {
	t.Helper()
	f := &integrationFixture{db: stageOneDatabase(t), sent: map[string]int{}}
	f.repo = pg.NewIntegrationRepository(f.db)
	f.jobs = pg.NewJobRepository(f.db)
	userRepo := pg.NewUserRepository(f.db)
	f.owner = users.User{ID: uuid.NewString(), Email: "owner@stage7.example", PasswordHash: "test-only-hash"}
	f.member = users.User{ID: uuid.NewString(), Email: "member@stage7.example", PasswordHash: "test-only-hash"}
	for _, user := range []users.User{f.owner, f.member} {
		if _, err := userRepo.Create(context.Background(), user); err != nil {
			t.Fatal(err)
		}
	}
	f.conferences = conferenceusecase.NewService(pg.NewConferenceRepository(f.db), userRepo, security.GenerateInviteCode)
	f.cipher, _ = security.NewProviderTokens(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer test-gateway-secret" {
			t.Error("missing gateway credentials")
		}
		if f.fail {
			w.WriteHeader(503)
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			t.Error("missing provider dedup key")
		}
		f.sent[key]++
		w.WriteHeader(204)
	}))
	t.Cleanup(server.Close)
	adapters, err := providers.NewIntegrations(providers.IntegrationConfig{Email: providers.AdapterConfig{Mode: "http", Endpoint: server.URL, Secret: "test-gateway-secret", AllowHTTP: true}, Push: providers.AdapterConfig{Mode: "http", Endpoint: server.URL, Secret: "test-gateway-secret", AllowHTTP: true}, Calendar: providers.AdapterConfig{Mode: "mock"}, MockConnectAllowed: true})
	if err != nil {
		t.Fatal(err)
	}
	f.service, err = u.NewService(f.repo, adapters, f.cipher, u.Options{PublicURL: "https://meet.example", MockConnectAllowed: true, ReminderOffsets: []time.Duration{24 * time.Hour, 15 * time.Minute}})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// drain выполняет все доступные integration jobs, исключая ещё не наступившие reminders и media queue.
// @parameters: t — контекст теста.
func (f *integrationFixture) drain(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for range 100 {
		count := 0
		for _, kind := range []string{"integrations.conference", "integrations.calendar", "integrations.event", "integrations.delivery"} {
			for {
				job, found, err := f.jobs.Claim(ctx, kind, time.Minute)
				if err != nil {
					t.Fatal(err)
				}
				if !found {
					break
				}
				count++
				err = f.service.Handle(ctx, job)
				state := "done"
				if errors.Is(err, jobs.ErrSkip) {
					state = "skipped"
				} else if err != nil {
					t.Fatal("job", kind, err)
				}
				if err = f.jobs.Finish(ctx, job, state, "", nil); err != nil {
					t.Fatal(err)
				}
			}
		}
		if count == 0 {
			return
		}
	}
	t.Fatal("integration queue did not drain")
}

// TestStageSevenIntegrationsPipeline проверяет scheduled→email/push/calendar, update/cancel, stable mappings и duplicate delivery.
// @parameters: t — контекст isolated PostgreSQL проверки.
func TestStageSevenIntegrationsPipeline(t *testing.T) {
	f := stageSevenIntegrations(t)
	ctx := context.Background()
	for _, user := range []users.User{f.owner, f.member} {
		prefs := d.DefaultPreferences(user.ID)
		prefs.Email = true
		prefs.Push = true
		if err := f.service.SavePreferences(ctx, user.ID, prefs); err != nil {
			t.Fatal(err)
		}
		device, err := f.service.RegisterDevice(ctx, user.ID, d.DeviceRequest{Platform: "web", Token: "sensitive-device-token-" + user.ID})
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(device)
		if strings.Contains(string(encoded), "sensitive") || strings.Contains(string(encoded), "Ciphertext") {
			t.Fatal("token exposed")
		}
	}
	if _, err := f.service.MockConnect(ctx, f.owner.ID); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Add(25 * time.Hour)
	c, err := f.conferences.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "<script>project</script>", ScheduledAt: &at})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.conferences.Join(ctx, f.member.ID, c.ID, conferences.JoinRequest{InviteCode: c.InviteCode}); err != nil {
		t.Fatal(err)
	}
	if err = f.service.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	mappings, err := f.service.CalendarMappings(ctx, f.owner.ID, c.ID)
	if err != nil || len(mappings) != 1 || mappings[0].ExternalEventID == "" || mappings[0].SyncStatus != "synced" {
		t.Fatal("calendar create", mappings, err)
	}
	external := mappings[0].ExternalEventID
	if _, err = f.service.CalendarMappings(ctx, f.member.ID, c.ID); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("participant calendar metadata leak", err)
	}
	var reminderCount int64
	if err = f.db.Table("background_jobs").Where("conference_id=? AND kind='integrations.event' AND payload->>'event'='conference.soon'", c.ID).Count(&reminderCount).Error; err != nil || reminderCount != 2 {
		t.Fatal("durable reminder schedule", reminderCount, err)
	}
	at = at.Add(time.Hour)
	if _, err = f.conferences.Schedule(ctx, f.owner.ID, c.ID, conferences.ScheduleRequest{ScheduledAt: at}); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	mappings, _ = f.service.CalendarMappings(ctx, f.owner.ID, c.ID)
	if len(mappings) != 1 || mappings[0].ExternalEventID != external || mappings[0].SourceVersion != 2 {
		t.Fatal("reschedule duplicated external event")
	}
	if _, err = f.conferences.Transition(ctx, f.owner.ID, c.ID, conferences.Cancelled); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	mappings, _ = f.service.CalendarMappings(ctx, f.owner.ID, c.ID)
	if mappings[0].ExternalEventID != external || mappings[0].SyncStatus != "cancelled" {
		t.Fatal("calendar cancellation missing")
	}
	// Force delivery redelivery after a crash boundary: gateway sees the same stable key, not a second unique delivery.
	var previous jobs.Job
	if err = f.db.Table("background_jobs").Where("kind='integrations.delivery' AND payload->>'channel'='email' AND state='done'").Order("created_at DESC").Take(&previous).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.db.Exec("UPDATE background_jobs SET state='queued',available_at=now(),finished_at=NULL WHERE id=?", previous.ID).Error; err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	f.mu.Lock()
	distinct := len(f.sent)
	f.mu.Unlock()
	if distinct < 8 {
		t.Fatal("email/push lifecycle not exercised", distinct)
	}
	var deliveries int64
	_ = f.db.Table("integration_deliveries").Where("job_id=?", previous.ID).Count(&deliveries).Error
	if deliveries != 1 {
		t.Fatal("duplicate delivery row")
	}
}

// TestStageSevenReminderStaleAndOAuthIsolation проверяет отсутствие просроченных 24h/15m reminders, state replay и revoke/refresh fencing.
// @parameters: t — контекст isolated DB.
func TestStageSevenReminderStaleAndOAuthIsolation(t *testing.T) {
	f := stageSevenIntegrations(t)
	ctx := context.Background()
	at := time.Now().UTC().Add(5 * time.Minute)
	c, err := f.conferences.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "Short notice", ScheduledAt: &at})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	var count int64
	_ = f.db.Table("background_jobs").Where("conference_id=? AND payload->>'event'='conference.soon'", c.ID).Count(&count).Error
	if count != 0 {
		t.Fatal("missed offsets replayed", count)
	}
	id := uuid.NewString()
	state := d.OAuthState{ID: id, UserID: f.owner.ID, Provider: "generic", StateHash: "state-hash", VerifierCiphertext: "encrypted-verifier", ExpiresAt: time.Now().Add(time.Minute)}
	if err = f.repo.SaveOAuthState(ctx, state); err != nil {
		t.Fatal(err)
	}
	if _, err = f.repo.TakeOAuthState(ctx, f.member.ID, "generic", "state-hash"); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("cross-user state accepted")
	}
	if _, err = f.repo.TakeOAuthState(ctx, f.owner.ID, "wrong", "state-hash"); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("cross-provider state accepted")
	}
	if _, err = f.repo.TakeOAuthState(ctx, f.owner.ID, "generic", "state-hash"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.repo.TakeOAuthState(ctx, f.owner.ID, "generic", "state-hash"); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("OAuth state replay")
	}
	connection := d.CalendarConnection{ID: uuid.NewString(), UserID: f.owner.ID, Provider: "generic", CalendarID: "primary", Status: "connected", AccessCiphertext: "access", RefreshCiphertext: "refresh"}
	if err = f.repo.SaveConnection(ctx, connection); err != nil {
		t.Fatal(err)
	}
	if err = f.repo.RevokeConnection(ctx, f.owner.ID, connection.ID); err != nil {
		t.Fatal(err)
	}
	connection.AccessCiphertext = "newaccess"
	if err = f.repo.RefreshConnection(ctx, connection, "refresh"); err == nil {
		t.Fatal("refresh revived revoked grant")
	}
	items, _ := f.repo.Connections(ctx, f.owner.ID)
	for _, item := range items {
		if item.ID == connection.ID && (item.Status != "revoked" || item.AccessCiphertext != "") {
			t.Fatal("revoked credentials persisted")
		}
	}
	// Per-conference advisory locks serialize both calendar job kinds without blocking a media/conference row.
	release, err := f.repo.AcquireCalendar(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.repo.AcquireCalendar(ctx, c.ID); err == nil {
		t.Fatal("calendar lock did not fence concurrent provider calls")
	}
	release()
	release2, err := f.repo.AcquireCalendar(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	release2()
	// A fake already-due reminder is ignored once the schedule generation changes.
	if err = f.db.Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,version,payload,dedup_key) VALUES('integrations.event',?,?,1,'{"event":"conference.soon","offsetSec":900}'::jsonb,?)`, c.ID, c.ID, "stale-reminder:"+c.ID).Error; err != nil {
		t.Fatal(err)
	}
	newAt := at.Add(time.Hour)
	if _, err = f.conferences.Schedule(ctx, f.owner.ID, c.ID, conferences.ScheduleRequest{ScheduledAt: newAt}); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	_ = f.db.Table("notifications").Where("type='conference.soon' AND payload->>'conferenceId'=?", c.ID).Count(&count).Error
	if count != 0 {
		t.Fatal("stale reminder delivered")
	}
}

// TestStageSevenIntegrationRoutesAuthorization проверяет Bearer identity, invalid UUID и отключённый OAuth без выдачи секретов.
// @parameters: t — контекст isolated API проверки.
func TestStageSevenIntegrationRoutesAuthorization(t *testing.T) {
	f := stageSevenIntegrations(t)
	gin.SetMode(gin.TestMode)
	tokens, _ := security.NewTokenService(strings.Repeat("stage-seven-auth-secret-", 3))
	token, _ := tokens.Issue(f.owner.ID)
	router := gin.New()
	httptransport.RegisterIntegrationRoutes(router, app.NewHandler(f.service), middleware.Authenticate(tokens))
	api := stageOneAPI{router: router}
	api.expect(t, "GET", "/integrations/capabilities", "", nil, 401, nil)
	api.expect(t, "GET", "/integrations/capabilities", token, nil, 200, nil)
	api.expect(t, "GET", "/notifications/preferences", token, nil, 200, nil)
	api.expect(t, "GET", "/integrations/calendars/generic/connect", token, nil, 503, nil)
	api.expect(t, "DELETE", "/integrations/calendars/not-a-uuid", token, nil, 422, nil)
	api.expect(t, "POST", "/integrations/calendars/mock", token, map[string]any{}, 201, nil)
	response := api.expect(t, "GET", "/integrations/calendars", token, nil, 200, nil)
	if strings.Contains(response.Body.String(), "ciphertext") {
		t.Fatal("credential projection leak")
	}
	api.expect(t, "POST", "/notifications/devices", token, map[string]any{"platform": "invalid", "token": "device-token"}, 422, nil)
	api.expect(t, "GET", "/conferences/"+uuid.NewString()+"/calendar", token, nil, 403, nil)
}

// TestStageSevenIntegrationFanoutAndProviderFailure проверяет durable continuation >100 получателей и изоляцию provider failure от расписания.
// @parameters: t — контекст isolated PostgreSQL.
func TestStageSevenIntegrationFanoutAndProviderFailure(t *testing.T) {
	f := stageSevenIntegrations(t)
	ctx := context.Background()
	at := time.Now().UTC().Add(time.Hour)
	c, err := f.conferences.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "Large fanout", ScheduledAt: &at})
	if err != nil {
		t.Fatal(err)
	}
	members := make([]users.User, 205)
	participants := make([]conferences.Participant, 205)
	for i := range members {
		id := uuid.NewString()
		members[i] = users.User{ID: id, Email: fmt.Sprintf("member-%d@stage7.example", i), PasswordHash: "test-only-hash"}
		participants[i] = conferences.Participant{ID: uuid.NewString(), ConferenceID: c.ID, UserID: &members[i].ID, Role: conferences.ParticipantRole, Status: conferences.Left, AdmissionState: conferences.AdmissionAdmitted, AdmissionVersion: 1, MediaPolicyVersion: 1, DisplayName: "Member"}
	}
	if err = f.db.Create(&members).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.db.Create(&participants).Error; err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	var notifications int64
	if err = f.db.Table("notifications").Where("type='conference.invited' AND payload->>'conferenceId'=?", c.ID).Count(&notifications).Error; err != nil || notifications != 206 {
		t.Fatal("bounded fanout lost or duplicated recipients", notifications, err)
	}
	var continuations int64
	_ = f.db.Table("background_jobs").Where("conference_id=? AND dedup_key LIKE 'integration-fanout:%'", c.ID).Count(&continuations).Error
	if continuations != 2 {
		t.Fatal("durable continuation missing", continuations)
	}
	prefs := d.DefaultPreferences(f.owner.ID)
	prefs.Email = true
	if err = f.service.SavePreferences(ctx, f.owner.ID, prefs); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.fail = true
	f.mu.Unlock()
	next := at.Add(time.Hour)
	if _, err = f.conferences.Schedule(ctx, f.owner.ID, c.ID, conferences.ScheduleRequest{ScheduledAt: next}); err != nil {
		t.Fatal(err)
	}
	job, found, err := f.jobs.Claim(ctx, "integrations.conference", time.Minute)
	if err != nil || !found {
		t.Fatal("schedule event absent", err)
	}
	if err = f.service.Handle(ctx, job); err != nil && !errors.Is(err, jobs.ErrSkip) {
		t.Fatal(err)
	}
	if err = f.jobs.Finish(ctx, job, "done", "", nil); err != nil {
		t.Fatal(err)
	}
	for {
		continuation, found, err := f.jobs.Claim(ctx, "integrations.event", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			break
		}
		if err = f.service.Handle(ctx, continuation); err != nil {
			t.Fatal(err)
		}
		if err = f.jobs.Finish(ctx, continuation, "done", "", nil); err != nil {
			t.Fatal(err)
		}
	}
	for range 300 {
		delivery, found, err := f.jobs.Claim(ctx, "integrations.delivery", time.Minute)
		if err != nil || !found {
			t.Fatal("email failure job absent", err)
		}
		err = f.service.Handle(ctx, delivery)
		if errors.Is(err, jobs.ErrSkip) {
			_ = f.jobs.Finish(ctx, delivery, "skipped", "", nil)
			continue
		}
		var classified jobs.Error
		if !errors.As(err, &classified) || !classified.Retryable {
			t.Fatal("provider error not bounded retry", err)
		}
		if err = f.service.FailJob(ctx, delivery, "provider_http"); err != nil {
			t.Fatal(err)
		}
		if err = f.jobs.Finish(ctx, delivery, "failed", "provider_http", nil); err != nil {
			t.Fatal(err)
		}
		var failed int64
		_ = f.db.Table("integration_deliveries").Where("job_id=? AND status='failed'", delivery.ID).Count(&failed).Error
		if failed != 1 {
			t.Fatal("terminal delivery missing")
		}
		actual, err := f.repo.Conference(ctx, c.ID)
		if err != nil || actual.Status != "scheduled" || actual.IntegrationVersion != 2 {
			t.Fatal("provider broke conference", actual, err)
		}
		return
	}
	t.Fatal("retryable email provider failure not reached")
}

// TestStageSevenOAuthEncryptedServerFlow проверяет state+S256 на полном серверном обмене, encrypted storage, refresh и revoke с fake HTTP.
// @parameters: t — контекст isolated DB, платные endpoints не используются.
func TestStageSevenOAuthEncryptedServerFlow(t *testing.T) {
	f := stageSevenIntegrations(t)
	ctx := context.Background()
	tokenCalls := 0
	revokeCalls := 0
	calendarCalls := 0
	var stateVerifier string
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokenCalls++
			_ = r.ParseForm()
			if r.Form.Get("client_secret") != "oauth-server-only-secret" {
				t.Error("OAuth client secret missing")
			}
			if r.Form.Get("grant_type") == "authorization_code" && r.Form.Get("code_verifier") != stateVerifier {
				t.Error("PKCE verifier mismatch")
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"access_token":"secret-access-token","refresh_token":"secret-refresh-token","token_type":"Bearer","expires_in":120,"scope":"calendar.events"}`)
		case "/revoke":
			revokeCalls++
			w.WriteHeader(200)
		case "/calendar/create", "/calendar/update":
			calendarCalls++
			var input struct {
				Event       d.CalendarEvent `json:"event"`
				AccessToken string          `json:"accessToken"`
			}
			if json.NewDecoder(r.Body).Decode(&input) != nil || input.AccessToken != "secret-access-token" {
				t.Error("calendar credential missing")
			}
			input.Event.ID = "gateway-stable-event"
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(input.Event)
		default:
			w.WriteHeader(404)
		}
	}))
	defer gateway.Close()
	adapters, err := providers.NewIntegrations(providers.IntegrationConfig{Calendar: providers.AdapterConfig{Mode: "http", Endpoint: gateway.URL, Secret: "gateway-server-secret", AllowHTTP: true}, OAuth: providers.OAuthConfig{AuthorizationURL: gateway.URL + "/authorize", TokenURL: gateway.URL + "/token", RevokeURL: gateway.URL + "/revoke", ClientID: "calendar-client", ClientSecret: "oauth-server-only-secret", RedirectURL: "https://meet.example/app/settings/calendar/generic/callback", Scopes: []string{"calendar.events"}, AllowHTTP: true}})
	if err != nil {
		t.Fatal(err)
	}
	service, err := u.NewService(f.repo, adapters, f.cipher, u.Options{PublicURL: "https://meet.example"})
	if err != nil {
		t.Fatal(err)
	}
	authorization, state, err := service.OAuthStart(ctx, f.owner.ID, "generic")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authorization)
	if err != nil {
		t.Fatal(err)
	}
	var storedState d.OAuthState
	if err = f.db.Where("user_id=?", f.owner.ID).Take(&storedState).Error; err != nil {
		t.Fatal(err)
	}
	stateVerifier, err = f.cipher.Decrypt(storedState.VerifierCiphertext, f.owner.ID+":oauth:"+storedState.ID)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(stateVerifier))
	if parsed.Query().Get("code_challenge") != base64.RawURLEncoding.EncodeToString(sum[:]) || parsed.Query().Get("code_challenge_method") != "S256" || storedState.StateHash == state || strings.Contains(storedState.VerifierCiphertext, stateVerifier) {
		t.Fatal("PKCE/state plaintext failure")
	}
	if _, err = service.OAuthCallback(ctx, f.member.ID, "generic", "one-time-code", state); !errors.Is(err, apperrors.ErrInvalidInput) || tokenCalls != 0 {
		t.Fatal("state owner bypass")
	}
	connection, err := service.OAuthCallback(ctx, f.owner.ID, "generic", "one-time-code", state)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(connection)
	if strings.Contains(string(encoded), "secret") || strings.Contains(connection.AccessCiphertext, "secret-access-token") || connection.RefreshCiphertext == "" {
		t.Fatal("OAuth token exposure")
	}
	if _, err = service.OAuthCallback(ctx, f.owner.ID, "generic", "same-code", state); !errors.Is(err, apperrors.ErrInvalidInput) || tokenCalls != 1 {
		t.Fatal("authorization code/state replay")
	}
	// Simulate approaching expiry: the worker refreshes in the same provider-boundary flow.
	if err = f.db.Model(&d.CalendarConnection{}).Where("id=?", connection.ID).Update("expires_at", time.Now().Add(10*time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Add(time.Hour)
	c, err := f.conferences.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "OAuth calendar", ScheduledAt: &at})
	if err != nil {
		t.Fatal(err)
	}
	f.service = service
	f.drain(t)
	if tokenCalls != 2 || calendarCalls != 1 {
		t.Fatal("refresh/calendar flow missing", tokenCalls, calendarCalls)
	}
	mappings, err := service.CalendarMappings(ctx, f.owner.ID, c.ID)
	if err != nil || len(mappings) != 1 || mappings[0].ExternalEventID != "gateway-stable-event" {
		t.Fatal("OAuth external mapping", mappings, err)
	}
	if err = service.Disconnect(ctx, f.owner.ID, connection.ID); err != nil {
		t.Fatal(err)
	}
	if revokeCalls != 1 {
		t.Fatal("external revoke missing")
	}
	items, _ := service.Connections(ctx, f.owner.ID)
	if len(items) != 1 || items[0].Status != "revoked" || items[0].AccessCiphertext != "" || items[0].RefreshCiphertext != "" {
		t.Fatal("OAuth tokens retained after revoke")
	}
}

// TestStageSevenRecordingCategorySuppressesLegacyFanout проверяет category preference в постоянной SQL вставке recording.ready.
// @parameters: t — контекст isolated PostgreSQL; defaults прежнего пользователя сохраняются.
func TestStageSevenRecordingCategorySuppressesLegacyFanout(t *testing.T) {
	f := stageSevenIntegrations(t)
	ctx := context.Background()
	c, err := f.conferences.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "Recording preference"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.conferences.Join(ctx, f.owner.ID, c.ID, conferences.JoinRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.conferences.Transition(ctx, f.owner.ID, c.ID, conferences.Active); err != nil {
		t.Fatal(err)
	}
	if _, err = f.conferences.Join(ctx, f.member.ID, c.ID, conferences.JoinRequest{InviteCode: c.InviteCode}); err != nil {
		t.Fatal(err)
	}
	prefs := d.DefaultPreferences(f.owner.ID)
	prefs.Recording = false
	prefs.Email = true
	prefs.Push = true
	if err = f.service.SavePreferences(ctx, f.owner.ID, prefs); err != nil {
		t.Fatal(err)
	}
	recordingRepo := pg.NewConferenceRecordingRepository(f.db)
	notificationRepo := pg.NewNotificationRepository(f.db).DisableLegacyReminders()
	recording, _, err := recordingRepo.Start(ctx, f.owner.ID, c.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Model(&records.Record{}).Where("id=?", recording.ID).Update("status", records.StatusReady).Error; err != nil {
		t.Fatal(err)
	}
	if err = notificationRepo.Generate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		userID string
		count  int64
	}{{f.owner.ID, 0}, {f.member.ID, 1}} {
		var count int64
		if err = f.db.Table("notifications").Where("user_id=? AND type='recording.ready' AND payload->>'recordingId'=?", check.userID, recording.UUID).Count(&count).Error; err != nil || count != check.count {
			t.Fatal("recording category ignored/default changed", check.userID, count, err)
		}
	}
	var count int64
	if err = f.db.Table("background_jobs").Where("user_id=? AND kind='integrations.delivery' AND conference_id=?", f.owner.ID, c.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("disabled recording created external delivery", count, err)
	}
	memberPrefs := d.DefaultPreferences(f.member.ID)
	memberPrefs.Recording = false
	if err = f.service.SavePreferences(ctx, f.member.ID, memberPrefs); err != nil {
		t.Fatal(err)
	}
	next, _, err := recordingRepo.Start(ctx, f.owner.ID, c.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Model(&records.Record{}).Where("id=?", next.ID).Update("status", records.StatusReady).Error; err != nil {
		t.Fatal(err)
	}
	if err = notificationRepo.Generate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Table("notifications").Where("type='recording.ready' AND payload->>'recordingId'=?", next.UUID).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("both disabled recipients received recording event", count, err)
	}
}

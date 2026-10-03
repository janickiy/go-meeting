package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	authapp "github.com/janickiy/go-recorder/internal/app/auth"
	conferencesapp "github.com/janickiy/go-recorder/internal/app/conferences"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/domain/users"
	postgresinfra "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	authusecase "github.com/janickiy/go-recorder/internal/usecase/auth"
	conferenceusecase "github.com/janickiy/go-recorder/internal/usecase/conferences"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const stageOneTestPassword = "stage-one-test-password"

// TestStageOnePostgres проверяет сценарий «этап один Postgres», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageOnePostgres(t *testing.T) {
	db := stageOneDatabase(t)
	gin.SetMode(gin.TestMode)
	userRepo := postgresinfra.NewUserRepository(db)
	conferenceRepo := postgresinfra.NewConferenceRepository(db)
	tokens, err := security.NewTokenService(strings.Repeat("integration-only-secret-", 3))
	if err != nil {
		t.Fatal(err)
	}
	authService, err := authusecase.NewService(userRepo, security.PasswordHasher{}, tokens)
	if err != nil {
		t.Fatal(err)
	}
	conferenceService := conferenceusecase.NewService(conferenceRepo, userRepo, security.GenerateInviteCode)
	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	httptransport.RegisterPlatformRoutes(router, authapp.NewHandler(authService), conferencesapp.NewHandler(conferenceService), httpmiddleware.Authenticate(tokens))
	api := stageOneAPI{router: router}
	owner, ownerToken := api.registerAndLogin(t, " Owner@Example.com ", " Owner ")
	member, memberToken := api.registerAndLogin(t, "member@example.com", "Member")
	ctx := context.Background()

	t.Run("authentication and lifecycle", /* Вложенный обработчик выполняет отдельный вариант тестового сценария с проверкой результата и очисткой ресурсов.

		@args
		  - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
		*/func(t *testing.T) {
			api.expect(t, "GET", "/auth/me", ownerToken, nil, 200, nil)
			api.expect(t, "POST", "/auth/logout", ownerToken, nil, 200, nil)
			api.expect(t, "GET", "/auth/me", ownerToken, nil, 200, nil) // Выход без серверного состояния сессии.
			api.expect(t, "POST", "/conferences", "", map[string]any{"title": "Test"}, 401, nil)
			api.expect(t, "POST", "/auth/register", "", map[string]any{"email": "OWNER@example.com", "password": stageOneTestPassword}, 409, nil)
			unknown := api.expect(t, "POST", "/auth/login", "", map[string]any{"email": "unknown@example.com", "password": stageOneTestPassword}, 401, nil)
			wrong := api.expect(t, "POST", "/auth/login", "", map[string]any{"email": owner.Email, "password": "wrong-password"}, 401, nil)
			if !bytes.Equal(unknown.Body.Bytes(), wrong.Body.Bytes()) {
				t.Fatal("unknown email and wrong password have different responses")
			}
			var created struct{ Item conferences.View }
			api.expect(t, "POST", "/conferences", ownerToken, map[string]any{"title": "  Stage one  "}, 201, &created)
			conference := created.Item
			if conference.OwnerID != owner.ID || conference.Status != conferences.Created || conference.Title != "Stage one" || conference.InviteCode == conference.ID || len(conference.InviteCode) != 32 {
				t.Fatal("incorrect conference defaults")
			}
			path := "/conferences/" + conference.ID
			var list struct{ Items []conferences.ParticipantView }
			api.expect(t, "GET", path+"/participants", ownerToken, nil, 200, &list)
			if len(list.Items) != 1 || list.Items[0].Role != conferences.Owner || list.Items[0].JoinedAt != nil || list.Items[0].LeftAt != nil || list.Items[0].Status != conferences.Left {
				t.Fatal("owner membership must not pretend that owner has joined")
			}
			ownerMembershipID := list.Items[0].ID
			api.expect(t, "POST", path+"/leave", ownerToken, nil, 200, nil)
			api.expect(t, "GET", path, memberToken, nil, 403, nil)
			api.expect(t, "GET", path+"/participants", memberToken, nil, 403, nil)
			api.expect(t, "POST", path+"/join", memberToken, nil, 403, nil)
			api.expect(t, "POST", "/conferences", ownerToken, map[string]any{"title": "Forged", "ownerId": member.ID}, 400, nil)
			api.expect(t, "POST", path+"/join", memberToken, map[string]any{"inviteCode": conference.InviteCode, "role": "owner"}, 400, nil)
			for _, action := range []string{"start", "finish", "cancel"} {
				api.expect(t, "POST", path+"/"+action, memberToken, nil, 403, nil)
			}
			var lookup struct{ Item map[string]any }
			api.expect(t, "GET", "/conference-invites/"+conference.InviteCode, memberToken, nil, 200, &lookup)
			if len(lookup.Item) != 3 || lookup.Item["id"] != conference.ID || lookup.Item["title"] != conference.Title || lookup.Item["status"] != "created" {
				t.Fatal("invite lookup exposes more than the safe preview")
			}
			var joined struct{ Item conferences.ParticipantView }
			api.expect(t, "POST", "/conference-invites/"+conference.InviteCode+"/join", memberToken, nil, 200, &joined)
			if joined.Item.Role != conferences.ParticipantRole || joined.Item.UserID == nil || *joined.Item.UserID != member.ID || joined.Item.JoinedAt == nil {
				t.Fatal("invite join did not create a regular membership")
			}
			api.expect(t, "POST", path+"/join", ownerToken, nil, 200, &joined)
			firstJoin := joined.Item
			if firstJoin.ID != ownerMembershipID || firstJoin.JoinedAt == nil || firstJoin.LeftAt != nil {
				t.Fatal("owner join replaced the membership or has incorrect dates")
			}
			firstJoinedAt := *firstJoin.JoinedAt // Последующее декодирование JSON повторно использует поля-указатели.
			api.expect(t, "POST", path+"/join", ownerToken, nil, 200, &joined)
			if !joined.Item.JoinedAt.Equal(firstJoinedAt) {
				t.Fatal("idempotent join changed joinedAt")
			}
			api.expect(t, "POST", path+"/finish", ownerToken, nil, 409, nil)
			api.expect(t, "POST", path+"/start", ownerToken, nil, 200, &created)
			if created.Item.StartedAt == nil || created.Item.FinishedAt != nil || created.Item.Status != conferences.Active {
				t.Fatal("invalid active conference dates")
			}
			api.expect(t, "POST", path+"/start", ownerToken, nil, 409, nil)
			api.expect(t, "POST", path+"/cancel", ownerToken, nil, 409, nil)
			api.expect(t, "POST", path+"/leave", ownerToken, nil, 200, &joined)
			if joined.Item.LeftAt == nil || joined.Item.Status != conferences.Left {
				t.Fatal("leave did not update membership")
			}
			api.expect(t, "POST", path+"/join", ownerToken, nil, 200, &joined)
			if joined.Item.ID != ownerMembershipID || joined.Item.LeftAt != nil || !joined.Item.JoinedAt.After(firstJoinedAt) {
				t.Fatal("rejoin must update dates on the same membership")
			}
			var conferencesList struct{ Items []conferences.View }
			api.expect(t, "GET", "/conferences", memberToken, nil, 200, &conferencesList)
			if len(conferencesList.Items) != 1 || conferencesList.Items[0].ID != conference.ID {
				t.Fatal("conference list must be scoped to membership")
			}
			api.expect(t, "POST", path+"/finish", ownerToken, nil, 200, &created)
			if created.Item.Status != conferences.Finished || created.Item.FinishedAt == nil {
				t.Fatal("finish did not close conference")
			}
			api.expect(t, "GET", path+"/participants", ownerToken, nil, 200, &list)
			for _, participant := range list.Items {
				if participant.Status != conferences.Left || participant.LeftAt == nil {
					t.Fatal("finish must close joined memberships")
				}
			}
			api.expect(t, "POST", path+"/join", memberToken, nil, 409, nil)
			api.expect(t, "POST", path+"/finish", ownerToken, nil, 409, nil)
			api.expect(t, "POST", path+"/leave", memberToken, nil, 200, nil)
			api.expect(t, "GET", "/conferences/invalid", ownerToken, nil, 422, nil)
			api.expect(t, "GET", "/conferences/"+uuid.NewString(), ownerToken, nil, 404, nil)
			api.expect(t, "GET", "/conference-invites/not-valid", memberToken, nil, 404, nil)
			api.expect(t, "GET", "/conferences?limit=101", ownerToken, nil, 422, nil)
			api.expect(t, "GET", "/conferences?offset=-1", ownerToken, nil, 422, nil)
			api.expect(t, "POST", "/conferences", ownerToken, map[string]any{"title": "   "}, 422, nil)
			api.expect(t, "POST", "/conferences", ownerToken, map[string]any{"title": "Cancelled"}, 201, &created)
			cancelPath := "/conferences/" + created.Item.ID
			api.expect(t, "POST", cancelPath+"/cancel", ownerToken, nil, 200, &created)
			if created.Item.Status != conferences.Cancelled || created.Item.StartedAt != nil || created.Item.FinishedAt == nil {
				t.Fatal("invalid cancelled conference dates")
			}
			api.expect(t, "POST", cancelPath+"/start", ownerToken, nil, 409, nil)
		})

	t.Run("cross-connection concurrency", /* Вложенный обработчик выполняет отдельный вариант тестового сценария с проверкой результата и очисткой ресурсов.

		@args
		  - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
		*/func(t *testing.T) {
			view, err := conferenceService.Create(ctx, owner.ID, conferences.CreateRequest{Title: "Concurrent"})
			if err != nil {
				t.Fatal(err)
			}
			memberModel, err := userRepo.GetByID(ctx, member.ID)
			if err != nil {
				t.Fatal(err)
			}
			const parallel = 12
			ids := make([]string, parallel)
			errs := make([]error, parallel)
			runConcurrent(parallel, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

				@args
				  - i (int): значение i типа int, используемое согласно назначению этой операции.
				*/func(i int) {
					p, err := conferenceRepo.Join(ctx, view.ID, memberModel, view.InviteCode)
					ids[i], errs[i] = p.ID, err
				})
			for i, err := range errs {
				if err != nil || ids[i] == "" || ids[i] != ids[0] {
					t.Fatalf("concurrent join failed or duplicated membership: %v", err)
				}
			}
			var count int64
			if err := db.Model(&conferences.Participant{}).Where("conference_id = ? AND user_id = ?", view.ID, member.ID).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("expected exactly one membership, count=%d, error=%v", count, err)
			}
			runConcurrent(parallel, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

				@args
				  - i (int): значение i типа int, используемое согласно назначению этой операции.
				*/func(i int) {
					_, errs[i] = conferenceRepo.Transition(ctx, view.ID, owner.ID, conferences.Active)
				})
			success := 0
			for _, err := range errs {
				if err == nil {
					success++
				} else if !errors.Is(err, apperrors.ErrConflict) {
					t.Fatalf("unexpected concurrent start error: %v", err)
				}
			}
			if success != 1 {
				t.Fatalf("concurrent start succeeded %d times, expected one", success)
			}
			runConcurrent(parallel, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

				@args
				  - i (int): значение i типа int, используемое согласно назначению этой операции.
				*/func(i int) {
					if i%2 == 0 {
						_, errs[i] = conferenceRepo.Join(ctx, view.ID, memberModel, "")
					} else {
						_, errs[i] = conferenceRepo.Transition(ctx, view.ID, owner.ID, conferences.Finished)
					}
				})
			for _, err := range errs {
				if err != nil && !errors.Is(err, apperrors.ErrConflict) {
					t.Fatalf("unexpected concurrent finish/join error: %v", err)
				}
			}
			final, err := conferenceRepo.Get(ctx, view.ID)
			p, membershipErr := conferenceRepo.Membership(ctx, view.ID, member.ID)
			if err != nil || membershipErr != nil || final.Status != conferences.Finished || p.Status != conferences.Left || p.LeftAt == nil {
				t.Fatal("join raced past terminal transition")
			}
			terminalRace, err := conferenceService.Create(ctx, owner.ID, conferences.CreateRequest{Title: "Finish versus cancel"})
			if err != nil {
				t.Fatal(err)
			}
			runConcurrent(parallel, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

				@args
				  - i (int): значение i типа int, используемое согласно назначению этой операции.
				*/func(i int) {
					target := conferences.Finished
					if i%2 == 0 {
						target = conferences.Cancelled
					}
					_, errs[i] = conferenceRepo.Transition(ctx, terminalRace.ID, owner.ID, target)
				})
			success = 0
			for _, err := range errs {
				if err == nil {
					success++
				} else if !errors.Is(err, apperrors.ErrConflict) {
					t.Fatalf("unexpected finish/cancel race error: %v", err)
				}
			}
			final, err = conferenceRepo.Get(ctx, terminalRace.ID)
			if err != nil || success != 1 || final.Status != conferences.Cancelled || final.StartedAt != nil || final.FinishedAt == nil {
				t.Fatal("finish/cancel race produced an inconsistent lifecycle")
			}
		})

	t.Run("constraints atomicity and legacy records", /* Вложенный обработчик выполняет отдельный вариант тестового сценария с проверкой результата и очисткой ресурсов.

		@args
		  - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
		*/func(t *testing.T) {
			view, err := conferenceService.Create(ctx, owner.ID, conferences.CreateRequest{Title: "Constraints"})
			if err != nil {
				t.Fatal(err)
			}
			collisionCode := view.InviteCode
			attempts := 0
			retryService := conferenceusecase.NewService(conferenceRepo, userRepo, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


				@return:
				  - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
				  - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func() (string, error) {
					attempts++
					if attempts == 1 {
						return collisionCode, nil
					}
					return security.GenerateInviteCode()
				})
			if _, err := retryService.Create(ctx, owner.ID, conferences.CreateRequest{Title: "Retry collision"}); err != nil || attempts != 2 {
				t.Fatalf("real unique invite collision was not retried: %v", err)
			}
			code, err := security.GenerateInviteCode()
			if err != nil {
				t.Fatal(err)
			}
			orphan := conferences.Conference{ID: uuid.NewString(), OwnerID: owner.ID, Title: "Must rollback", InviteCode: code, Status: conferences.Created}
			missingUser := uuid.NewString()
			invalidOwner := conferences.Participant{ID: uuid.NewString(), ConferenceID: orphan.ID, UserID: &missingUser, DisplayName: "Missing", Role: conferences.Owner, Status: conferences.Left}
			if _, err := conferenceRepo.Create(ctx, orphan, invalidOwner); err == nil {
				t.Fatal("invalid owner unexpectedly accepted")
			}
			if _, err := conferenceRepo.Get(ctx, orphan.ID); !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatal("conference creation was not atomic")
			}
			for i := 0; i < 2; i++ {
				guest := conferences.Participant{ID: uuid.NewString(), ConferenceID: view.ID, DisplayName: "Future guest", Role: conferences.Guest, Status: conferences.Left}
				if err := db.Create(&guest).Error; err != nil {
					t.Fatalf("nullable guest identity uniqueness is incorrect: %v", err)
				}
			}
			ownerID := owner.ID
			duplicate := conferences.Participant{ID: uuid.NewString(), ConferenceID: view.ID, UserID: &ownerID, DisplayName: "Duplicate", Role: conferences.ParticipantRole, Status: conferences.Left}
			if err := db.Create(&duplicate).Error; err == nil {
				t.Fatal("duplicate non-null membership accepted")
			}
			if err := db.Exec("DELETE FROM users WHERE id = ?", owner.ID).Error; err == nil {
				t.Fatal("owner foreign key must restrict deletion")
			}
			legacy := records.Record{UUID: uuid.NewString(), ConferenceID: uuid.NewString(), RequestedBy: &ownerID, SourceType: "browser", TransportType: "webrtc", Status: "starting", QualityMode: "auto", SegmentDurationSec: 5}
			if _, err := postgresinfra.NewRecordRepository(db).Create(ctx, legacy); err != nil {
				t.Fatalf("legacy recorder UUIDs must remain valid: %v", err)
			}
			stored, err := userRepo.GetByID(ctx, owner.ID)
			if err != nil || !strings.HasPrefix(stored.PasswordHash, "$argon2id$") {
				t.Fatal("database does not contain an Argon2id hash")
			}
			valid, err := (security.PasswordHasher{}).Verify(stageOneTestPassword, stored.PasswordHash)
			if err != nil || !valid {
				t.Fatal("database does not contain a valid Argon2id hash")
			}
			var sqlLog bytes.Buffer
			verboseUsers := postgresinfra.NewUserRepository(db.Session(&gorm.Session{Logger: logger.New(log.New(&sqlLog, "", 0), logger.Config{LogLevel: logger.Info})}))
			stored.ID = uuid.NewString()
			if _, err := verboseUsers.Create(ctx, stored); !errors.Is(err, apperrors.ErrConflict) {
				t.Fatal("expected duplicate email conflict")
			}
			if sqlLog.Len() != 0 {
				t.Fatal("user repository allowed credential-bearing SQL logging")
			}
			registrationErrors := make([]error, 12)
			runConcurrent(len(registrationErrors), /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

				@args
				  - i (int): значение i типа int, используемое согласно назначению этой операции.
				*/func(i int) {
					_, registrationErrors[i] = userRepo.Create(ctx, users.User{ID: uuid.NewString(), Email: "concurrent-registration@example.com", PasswordHash: stored.PasswordHash})
				})
			registrations := 0
			for _, err := range registrationErrors {
				if err == nil {
					registrations++
				} else if !errors.Is(err, apperrors.ErrConflict) {
					t.Fatalf("unexpected concurrent registration error: %v", err)
				}
			}
			if registrations != 1 {
				t.Fatalf("concurrent duplicate email registered %d times", registrations)
			}
		})
}

// stageOneAPI хранит изолированное состояние тестового компонента «этап один API».
// @params:
//   - router: значение router типа *gin.Engine, используемое согласно назначению этой операции.
type stageOneAPI struct{ router *gin.Engine }

// expect подготавливает или проверяет часть тестового сценария «expect».
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//   - method (string): значение method типа string, используемое согласно назначению этой операции.
//   - path (string): путь к локальному файлу или каталогу операции.
//   - token (string): подписанный токен или токен владения, который необходимо проверить.
//   - body (any): тело входящего запроса или сериализованные данные передачи.
//   - status (int): состояние ресурса, ответа или фильтра выборки.
//   - response (any): значение response типа any, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (*httptest.ResponseRecorder): значение, подготовленное операцией для вызывающей стороны.
func (a stageOneAPI) expect(t *testing.T, method, path, token string, body any, status int, response any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	a.router.ServeHTTP(w, req)
	for _, credential := range []string{stageOneTestPassword, "passwordHash", "password_hash", "$argon2id$"} {
		if strings.Contains(w.Body.String(), credential) {
			t.Fatal("credentials leaked in HTTP response")
		}
	}
	if w.Code != status {
		t.Fatalf("%s %s: expected %d, got %d: %s", method, path, status, w.Code, w.Body.String())
	}
	if response != nil {
		if err := json.Unmarshal(w.Body.Bytes(), response); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

// registerAndLogin подготавливает или проверяет часть тестового сценария «register и Login».
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//   - email (string): адрес электронной почты пользователя.
//   - name (string): имя поля, компонента или ресурса, используемое в операции.
//
// @return:
//   - результат 1 (users.View): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (string): значение, подготовленное операцией для вызывающей стороны.
func (a stageOneAPI) registerAndLogin(t *testing.T, email, name string) (users.View, string) {
	t.Helper()
	var registered struct{ User users.View }
	a.expect(t, "POST", "/auth/register", "", map[string]any{"email": email, "password": stageOneTestPassword, "displayName": name}, 201, &registered)
	if registered.User.Email != strings.ToLower(strings.TrimSpace(email)) || registered.User.DisplayName == nil || *registered.User.DisplayName != strings.TrimSpace(name) {
		t.Fatal("registration normalization failed")
	}
	var login users.LoginResponse
	a.expect(t, "POST", "/auth/login", "", map[string]any{"email": email, "password": stageOneTestPassword}, 200, &login)
	if login.AccessToken == "" || login.ExpiresIn != 3600 || login.TokenType != "Bearer" || login.User.ID != registered.User.ID {
		t.Fatal("invalid login response")
	}
	return registered.User, login.AccessToken
}

// runConcurrent подготавливает или проверяет часть тестового сценария «выполнение одновременный».
//
// @args
//   - count (int): значение count типа int, используемое согласно назначению этой операции.
//   - fn (func(int)): вызываемый обработчик «fn» с контрактом, указанным в типе.
func runConcurrent(count int, fn func(int)) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < count; i++ {
		wg.Add(1)
		go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - i (int): значение i типа int, используемое согласно назначению этой операции.
		*/func(i int) {
			defer wg.Done()
			<-start
			fn(i)
		}(i)
	}
	close(start)
	wg.Wait()
}

// stageOneDatabase подготавливает или проверяет часть тестового сценария «этап один Database».
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//
// @return:
//   - результат 1 (*gorm.DB): значение, подготовленное операцией для вызывающей стороны.
func stageOneDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("RECORDER_STAGE1_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set RECORDER_STAGE1_TEST_POSTGRES_DSN to a local PostgreSQL URL with CREATE DATABASE permission")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || (parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1") {
		t.Fatal("stage one integration tests require a local PostgreSQL URL")
	}
	admin, err := postgresinfra.Connect(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin = admin.Session(&gorm.Session{Logger: logger.Discard})
	adminSQL, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() { _ = adminSQL.Close() })
	databaseName := "go_recorder_stage1_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE DATABASE " + databaseName).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() {
			if err := admin.Exec("DROP DATABASE " + databaseName + " WITH (FORCE)").Error; err != nil {
				t.Errorf("drop isolated stage one database: %v", err)
			}
		})
	parsed.Path = "/" + databaseName
	db, err := postgresinfra.Connect(parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	db = db.Session(&gorm.Session{Logger: logger.Discard})
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(20)
	t.Cleanup( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() { _ = sqlDB.Close() })
	migrations := filepath.Join("..", "..", "database", "migrations")
	for i := 0; i < 2; i++ { // Повторный запуск миграций при старте предусмотрен намеренно.
		if err := postgresinfra.RunMigrations(db, migrations); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

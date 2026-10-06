package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	authapp "github.com/janickiy/go-recorder/internal/app/auth"
	conferencesapp "github.com/janickiy/go-recorder/internal/app/conferences"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	authcase "github.com/janickiy/go-recorder/internal/usecase/auth"
	"gorm.io/gorm"
)

const persistentTestSecret = "persistent-session-integration-secret-32bytes"

func persistentFixture(t *testing.T) (*gorm.DB, *authcase.Service, *security.TokenService, users.User, *gin.Engine) {
	t.Helper()
	db := stageOneDatabase(t)
	repo := pg.NewUserRepository(db)
	hash, err := (security.PasswordHasher{}).Hash(stageOneTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	account, err := repo.Create(context.Background(), users.User{ID: uuid.NewString(), Email: "persistent@example.test", PasswordHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := security.NewTokenService(persistentTestSecret)
	if err != nil {
		t.Fatal(err)
	}
	service, err := authcase.NewService(repo, security.PasswordHasher{}, tokens)
	if err != nil {
		t.Fatal(err)
	}
	service.WithSessions(pg.NewAuthSessionRepository(db))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	httptransport.RegisterPlatformRoutes(router, authapp.NewHandler(service).WithSessionCookies("https://meeting.example.test", true), conferencesapp.NewHandler(nil), middleware.Authenticate(service))
	return db, service, tokens, account, router
}

func persistentRequest(router *gin.Engine, method, path, access string, cookie *http.Cookie, origin, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "https://meeting.example.test/api/v1/auth"+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if access != "" {
		request.Header.Set("Authorization", "Bearer "+access)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func persistentLogin(t *testing.T, router *gin.Engine) (users.LoginResponse, *http.Cookie) {
	t.Helper()
	result := persistentRequest(router, "POST", "/login", "", nil, "https://meeting.example.test", `{"email":"persistent@example.test","password":"`+stageOneTestPassword+`"}`)
	if result.Code != 200 {
		t.Fatalf("login: %d %s", result.Code, result.Body.String())
	}
	var response users.LoginResponse
	if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	cookies := result.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing browser session cookie")
	}
	cookie := cookies[0]
	if cookie.Name != authapp.SessionCookieName || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/api/v1/auth" || cookie.Domain != "" || cookie.MaxAge != 400*24*60*60 {
		t.Fatal("browser session cookie policy is incomplete")
	}
	if strings.Contains(result.Body.String(), cookie.Value) || strings.Contains(result.Body.String(), "SessionToken") || result.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("session secret leaked into cacheable JSON")
	}
	return response, cookie
}

func TestPersistentAuthSurvivesJWTExpiryIdleAndAPIRecreation(t *testing.T) {
	db, service, tokens, account, router := persistentFixture(t)
	login, cookie := persistentLogin(t, router)
	_, _, sid, expiry, err := tokens.VerifyAuthorization(login.AccessToken)
	if err != nil || sid == "" || expiry.Sub(time.Now()) > time.Hour || login.ExpiresIn != 3600 {
		t.Fatal("access token must stay short-lived and session-bound", err)
	}
	if err := db.Model(&users.AuthSession{}).Where("id = ?", sid).Update("created_at", time.Now().AddDate(-10, 0, 0)).Error; err != nil {
		t.Fatal(err)
	}
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": account.ID, "iss": "go-recorder", "aud": "go-recorder-api", "sid": sid,
		"iat": time.Now().Add(-2 * time.Hour).Unix(), "exp": time.Now().Add(-time.Hour).Unix(),
	}).SignedString([]byte(persistentTestSecret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Verify(expired); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("expired access token accepted", err)
	}
	// Simulate a new API instance; durable login comes from PostgreSQL, not process memory.
	restarted, err := authcase.NewService(pg.NewUserRepository(db), security.PasswordHasher{}, tokens)
	if err != nil {
		t.Fatal(err)
	}
	restarted.WithSessions(pg.NewAuthSessionRepository(db))
	refreshed, err := restarted.Refresh(context.Background(), cookie.Value)
	if err != nil || refreshed.User.ID != account.ID || refreshed.SessionToken != cookie.Value {
		t.Fatal("idle sign-in did not survive API restart", err)
	}
	response := persistentRequest(router, "POST", "/refresh", expired, cookie, "https://meeting.example.test", `{}`)
	if response.Code != 200 {
		t.Fatal("expired JWT prevented cookie refresh", response.Code)
	}
	var renewed users.LoginResponse
	if err := json.Unmarshal(response.Body.Bytes(), &renewed); err != nil {
		t.Fatal(err)
	}
	if got := response.Result().Cookies()[0]; got.Value != cookie.Value || got.MaxAge != cookie.MaxAge {
		t.Fatal("refresh changed durable session secret")
	}
	if persistentRequest(router, "GET", "/me", renewed.AccessToken, nil, "", "").Code != 200 {
		t.Fatal("renewed JWT failed authentication")
	}
	// Logging out this browser does not revoke a separately signed-in browser.
	other, otherCookie := persistentLogin(t, router)
	logout := persistentRequest(router, "POST", "/logout", expired, cookie, "https://meeting.example.test", `{}`)
	if logout.Code != 200 || logout.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("expired JWT blocked explicit logout", logout.Code)
	}
	if persistentRequest(router, "POST", "/refresh", "", cookie, "https://meeting.example.test", `{}`).Code != 401 {
		t.Fatal("logged-out cookie refreshed")
	}
	if persistentRequest(router, "GET", "/me", renewed.AccessToken, nil, "", "").Code != 401 {
		t.Fatal("logout did not revoke existing SID JWT")
	}
	if err := service.AuthorizeSession(context.Background(), account.ID, sid); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("realtime SID remained authorized after logout", err)
	}
	if persistentRequest(router, "POST", "/refresh", "", otherCookie, "https://meeting.example.test", `{}`).Code != 200 || persistentRequest(router, "GET", "/me", other.AccessToken, nil, "", "").Code != 200 {
		t.Fatal("logout revoked another browser")
	}
}

func TestPersistentAuthOriginGuestAndInvalidCookieBoundaries(t *testing.T) {
	db, service, tokens, account, router := persistentFixture(t)
	login, cookie := persistentLogin(t, router)
	for _, path := range []string{"/login", "/refresh", "/session", "/logout"} {
		result := persistentRequest(router, "POST", path, login.AccessToken, cookie, "https://evil.example.test", `{}`)
		if result.Code != 403 || len(result.Result().Cookies()) != 0 {
			t.Fatalf("foreign Origin accepted by %s: %d", path, result.Code)
		}
	}
	request := httptest.NewRequest("POST", "https://meeting.example.test/api/v1/auth/logout", strings.NewReader(`{}`))
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal("cross-site metadata accepted")
	}
	request.Header.Set("Sec-Fetch-Site", "same-site")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal("originless same-site browser request accepted")
	}
	request = httptest.NewRequest("POST", "https://meeting.example.test/api/v1/auth/refresh", strings.NewReader(`{}`))
	request.Header.Set("Origin", "https://meeting.example.test")
	request.Header.Set("Content-Type", "text/plain")
	request.AddCookie(cookie)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 415 {
		t.Fatal("simple content type accepted for cookie mutation")
	}
	if _, err := service.Refresh(context.Background(), cookie.Value); err != nil {
		t.Fatal("CSRF attempts changed valid session", err)
	}
	invalid := *cookie
	invalid.Value = strings.Repeat("A", 43)
	if persistentRequest(router, "POST", "/refresh", "", &invalid, "", `{}`).Code != 401 {
		t.Fatal("forged cookie accepted")
	}
	conferenceID := uuid.NewString()
	guest := users.User{ID: uuid.NewString(), Email: "guest@example.test", PasswordHash: "guest-only", GuestConferenceID: &conferenceID}
	if err := db.Create(&guest).Error; err != nil {
		t.Fatal(err)
	}
	guestToken, err := tokens.IssueGuest(guest.ID, conferenceID)
	if err != nil {
		t.Fatal(err)
	}
	if persistentRequest(router, "POST", "/session", guestToken, nil, "", `{}`).Code != 403 {
		t.Fatal("guest obtained an account session")
	}
	if _, err := service.Bootstrap(context.Background(), guest.ID, "", guestToken); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("guest bootstrap bypassed service boundary", err)
	}
	if err := service.AuthorizeSession(context.Background(), account.ID, uuid.NewString()); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("unknown SID accepted", err)
	}
	// Already signed-in tabs reuse their current cookie and SID.
	result := persistentRequest(router, "POST", "/session", login.AccessToken, cookie, "", `{}`)
	if result.Code != 200 || result.Result().Cookies()[0].Value != cookie.Value {
		t.Fatal("existing tab created a replacement cookie")
	}
	if _, err := service.Bootstrap(context.Background(), account.ID, "", login.AccessToken); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("stolen SID access token minted a durable cookie without its refresh credential", err)
	}
	_, otherCookie := persistentLogin(t, router)
	if _, err := service.Bootstrap(context.Background(), account.ID, otherCookie.Value, login.AccessToken); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("SID access token bypassed the matching-cookie boundary", err)
	}
	if err := service.Logout(context.Background(), cookie.Value, login.AccessToken); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Bootstrap(context.Background(), account.ID, "", login.AccessToken); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("SID bootstrap resurrected an explicitly revoked session", err)
	}
}

func TestPersistentAuthLegacyUpgradeConcurrencyAndLogoutMarkers(t *testing.T) {
	db, service, tokens, account, _ := persistentFixture(t)
	ctx := context.Background()
	legacy, err := tokens.Issue(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	responses := make([]users.LoginResponse, 12)
	errs := make([]error, len(responses))
	var wait sync.WaitGroup
	for i := range responses {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			responses[index], errs[index] = service.Bootstrap(ctx, account.ID, "", legacy)
		}(i)
	}
	wait.Wait()
	var sharedSID string
	for i, response := range responses {
		if errs[i] != nil {
			t.Fatal("concurrent legacy upgrade failed", errs[i])
		}
		_, _, sid, _, err := tokens.VerifyAuthorization(response.AccessToken)
		if err != nil || sid == "" {
			t.Fatal("missing SID in upgraded JWT", err)
		}
		if sharedSID == "" {
			sharedSID = sid
		}
		if sid != sharedSID {
			t.Fatal("concurrent tabs created multiple sessions")
		}
		if _, err := service.Refresh(ctx, response.SessionToken); err != nil {
			t.Fatal("concurrent tab cookie became invalid", err)
		}
	}
	var count int64
	if err := db.Model(&users.AuthSession{}).Where("user_id = ?", account.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("legacy upgrade was not idempotent", count, err)
	}
	reused, err := service.Bootstrap(ctx, account.ID, responses[0].SessionToken, legacy)
	if err != nil || reused.SessionToken != responses[0].SessionToken {
		t.Fatal("legacy tab did not reuse its cookie", err)
	}
	if err := service.Logout(ctx, responses[0].SessionToken, legacy); err != nil {
		t.Fatal(err)
	}
	for _, response := range responses {
		if _, err := service.Refresh(ctx, response.SessionToken); !errors.Is(err, apperrors.ErrUnauthorized) {
			t.Fatal("logout did not revoke all cookies of shared session", err)
		}
	}
	if _, err := service.Bootstrap(ctx, account.ID, "", legacy); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("late legacy upgrade resurrected logout", err)
	}
	if _, err := service.Verify(legacy); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("legacy JWT remained valid after explicit logout", err)
	}
	// A logout that wins before any upgrade still leaves a durable marker.
	second, err := pg.NewUserRepository(db).Create(ctx, users.User{ID: uuid.NewString(), Email: "marker@example.test", PasswordHash: account.PasswordHash})
	if err != nil {
		t.Fatal(err)
	}
	beforeUpgrade, err := tokens.Issue(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Logout(ctx, "", beforeUpgrade); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Bootstrap(ctx, second.ID, "", beforeUpgrade); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("logout-before-upgrade marker was ignored", err)
	}
	// Start upgrade and logout at the same instant; neither transaction ordering may
	// leave a usable browser session after logout has completed.
	for range 6 {
		racer, err := pg.NewUserRepository(db).Create(ctx, users.User{ID: uuid.NewString(), Email: "racer-" + uuid.NewString() + "@example.test", PasswordHash: account.PasswordHash})
		if err != nil {
			t.Fatal(err)
		}
		access, err := tokens.Issue(racer.ID)
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var upgraded users.LoginResponse
		var upgradeErr, logoutErr error
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			upgraded, upgradeErr = service.Bootstrap(ctx, racer.ID, "", access)
		}()
		go func() { defer wait.Done(); <-start; logoutErr = service.Logout(ctx, "", access) }()
		close(start)
		wait.Wait()
		if logoutErr != nil || upgradeErr != nil && !errors.Is(upgradeErr, apperrors.ErrUnauthorized) {
			t.Fatal("concurrent logout/upgrade failed", logoutErr, upgradeErr)
		}
		if upgradeErr == nil {
			if _, err := service.Refresh(ctx, upgraded.SessionToken); !errors.Is(err, apperrors.ErrUnauthorized) {
				t.Fatal("concurrent logout/upgrade left a usable cookie", err)
			}
		}
		if _, err := service.Bootstrap(ctx, racer.ID, "", access); !errors.Is(err, apperrors.ErrUnauthorized) {
			t.Fatal("concurrent logout marker did not prevent later upgrade", err)
		}
	}
	// A fresh password login is allowed after explicit logout.
	fresh, err := service.Login(ctx, users.LoginRequest{Email: account.Email, Password: stageOneTestPassword})
	if err != nil {
		t.Fatal("explicit new login was blocked by old marker", err)
	}
	if _, err := service.Verify(fresh.AccessToken); err != nil {
		t.Fatal("fresh sign-in did not authorize", err)
	}
}

package api_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	authapp "github.com/janickiy/go-recorder/internal/app/auth"
	conferencesapp "github.com/janickiy/go-recorder/internal/app/conferences"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/ratelimit"
	"github.com/janickiy/go-recorder/internal/domain/users"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	authusecase "github.com/janickiy/go-recorder/internal/usecase/auth"
)

const authTestSecret = "unit-test-secret-at-least-32-bytes-long"

func authRouter(t *testing.T, limiter httpmiddleware.Limiter) (*gin.Engine, *authRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := &authRepository{}
	tokens, err := security.NewTokenService(authTestSecret)
	if err != nil {
		t.Fatal(err)
	}
	service, err := authusecase.NewService(repo, authPasswords{}, tokens)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	if limiter != nil {
		router.Use(httpmiddleware.RateLimit(limiter, httpmiddleware.RateLimitConfig{Enabled: true, Rules: []httpmiddleware.Rule{{Method: "POST", Path: "/api/v1/auth/login", Scope: "ip", Limit: 2, Window: time.Minute, Key: httpmiddleware.ClientIPKey}}}))
	}
	httptransport.RegisterPlatformRoutes(router, authapp.NewHandler(service), conferencesapp.NewHandler(nil), httpmiddleware.Authenticate(tokens))
	return router, repo
}

func TestAuthRegistrationValidationAndNoSecrets(t *testing.T) {
	router, repo := authRouter(t, nil)
	response := performJSON(router, "POST", "/api/v1/auth/register", `{"email":" TEST@Example.com ","password":"StrongPassword123","displayName":" Test "}`)
	assertStatus(t, response.Code, 201)
	if repo.user.Email != "test@example.com" || repo.user.DisplayName == nil || *repo.user.DisplayName != "Test" {
		t.Fatal("user was not normalized")
	}
	assertNoCredentials(t, response.Body.String())
	response = performJSON(router, "POST", "/api/v1/auth/register", `{"email":"test@example.com","password":"StrongPassword123"}`)
	assertStatus(t, response.Code, 409)
	for _, body := range []string{`{"email":"bad","password":"StrongPassword123"}`, `{"email":"valid@example.com","password":"short"}`} {
		assertStatus(t, performJSON(router, "POST", "/api/v1/auth/register", body).Code, 422)
	}
	for _, body := range []string{`{"email":"valid@example.com","password":"StrongPassword123","userId":"forged"}`, `{} {}`, "{"} {
		assertStatus(t, performJSON(router, "POST", "/api/v1/auth/register", body).Code, 400)
	}
}

func TestAuthLoginMeAndStatelessLogout(t *testing.T) {
	router, _ := authRouter(t, nil)
	performJSON(router, "POST", "/api/v1/auth/register", `{"email":"user@example.com","password":"StrongPassword123"}`)
	response := performJSON(router, "POST", "/api/v1/auth/login", `{"email":"USER@example.com","password":"StrongPassword123"}`)
	assertStatus(t, response.Code, 200)
	var login users.LoginResponse
	if err := json.Unmarshal(response.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}
	if login.TokenType != "Bearer" || login.ExpiresIn != 3600 || login.AccessToken == "" {
		t.Fatal("token response is incomplete")
	}
	assertNoCredentials(t, response.Body.String())
	wrong := performJSON(router, "POST", "/api/v1/auth/login", `{"email":"user@example.com","password":"wrong-password"}`)
	unknown := performJSON(router, "POST", "/api/v1/auth/login", `{"email":"unknown@example.com","password":"wrong-password"}`)
	if wrong.Code != 401 || unknown.Code != 401 || wrong.Body.String() != unknown.Body.String() {
		t.Fatal("login leaks whether an email exists")
	}
	for _, route := range []struct{ method, path string }{{"GET", "/api/v1/auth/me"}, {"POST", "/api/v1/auth/logout"}} {
		request := httptest.NewRequest(route.method, route.path, nil)
		request.Header.Set("Authorization", "Bearer "+login.AccessToken)
		result := httptest.NewRecorder()
		router.ServeHTTP(result, request)
		assertStatus(t, result.Code, 200)
		assertNoCredentials(t, result.Body.String())
	}
	request := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	request.Header.Set("Authorization", "Bearer "+login.AccessToken)
	result := httptest.NewRecorder()
	router.ServeHTTP(result, request)
	assertStatus(t, result.Code, 200)
}

func TestAuthPasswordCharacterPolicy(t *testing.T) {
	cases := []struct {
		name, password string
		status         int
	}{
		{"eight latin characters", "abcdefgh", 201},
		{"digits without composition rules", "12345678", 201},
		{"eight cyrillic characters", "абвгдежз", 201},
		{"128 cyrillic characters", strings.Repeat("я", 128), 201},
		{"128 supplementary unicode characters", strings.Repeat("😀", 128), 201},
		{"seven latin characters", "abcdefg", 422},
		{"seven cyrillic characters", "абвгдеж", 422},
		{"129 cyrillic characters", strings.Repeat("я", 129), 422},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			router, _ := authRouter(t, nil)
			body, err := json.Marshal(users.RegisterRequest{Email: "policy@example.com", Password: test.password})
			if err != nil {
				t.Fatal(err)
			}
			assertStatus(t, performJSON(router, "POST", "/api/v1/auth/register", string(body)).Code, test.status)
			if test.status == 201 {
				loginBody, err := json.Marshal(users.LoginRequest{Email: "policy@example.com", Password: test.password})
				if err != nil {
					t.Fatal(err)
				}
				assertStatus(t, performJSON(router, "POST", "/api/v1/auth/login", string(loginBody)).Code, 200)
			}
		})
	}
}

func TestAuthExistingShortUnicodePasswordStillWorks(t *testing.T) {
	router, repo := authRouter(t, nil)
	// Six Cyrillic characters met the old 12-byte registration minimum.
	repo.user = users.User{ID: uuid.NewString(), Email: "legacy@example.com", PasswordHash: "test-hash:пароль"}
	assertStatus(t, performJSON(router, "POST", "/api/v1/auth/login", `{"email":"legacy@example.com","password":"пароль"}`).Code, 200)
}

func TestPlatformRoutesRequireBearerAuthentication(t *testing.T) {
	router, _ := authRouter(t, nil)
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject: uuid.NewString(), Issuer: "go-recorder", Audience: jwt.ClaimStrings{"go-recorder-api"},
		IssuedAt: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)), ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
	}).SignedString([]byte(authTestSecret))
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"", "Basic abc", "Bearer invalid-token", "Bearer a b", "Bearer " + expired} {
		for _, path := range []string{"/api/v1/conferences", "/api/v1/conference-invites/any/join", "/api/v1/auth/logout"} {
			request := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
			request.Header.Set("Authorization", header)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			assertStatus(t, response.Code, 401)
		}
	}
}

func TestLoginRateLimitCannotBeBypassedWithForwardedIP(t *testing.T) {
	limiter := &loginLimiter{}
	router, _ := authRouter(t, limiter)
	for attempt := range 3 {
		request := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"email":"unknown@example.com","password":"wrong-password"}`))
		request.RemoteAddr = "192.0.2.1:12345"
		request.Header.Set("X-Forwarded-For", []string{"198.51.100.1", "198.51.100.2", "198.51.100.3"}[attempt])
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		want := 401
		if attempt == 2 {
			want = 429
		}
		assertStatus(t, response.Code, want)
	}
	if len(limiter.keys) != 1 {
		t.Fatal("forwarded headers bypassed login throttling")
	}
}

func assertNoCredentials(t *testing.T, raw string) {
	t.Helper()
	if strings.Contains(raw, "password") || strings.Contains(raw, "Password") || strings.Contains(raw, "StrongPassword123") || strings.Contains(raw, "$argon2") {
		// The intentional generic login error is checked separately.
		t.Fatalf("response exposed credential fields: %s", raw)
	}
}

type authRepository struct{ user users.User }

func (r *authRepository) Create(_ context.Context, user users.User) (users.User, error) {
	if r.user.Email == user.Email {
		return users.User{}, apperrors.New(apperrors.ErrConflict, "email is already registered")
	}
	r.user = user
	return user, nil
}
func (r *authRepository) GetByID(_ context.Context, id string) (users.User, error) {
	if r.user.ID == id && id != "" {
		return r.user, nil
	}
	return users.User{}, apperrors.ErrNotFound
}
func (r *authRepository) GetByEmail(_ context.Context, email string) (users.User, error) {
	if r.user.Email == email && email != "" {
		return r.user, nil
	}
	return users.User{}, apperrors.ErrNotFound
}

type authPasswords struct{}

func (authPasswords) Hash(password string) (string, error) { return "test-hash:" + password, nil }
func (authPasswords) Verify(password, hash string) (bool, error) {
	return hash == "test-hash:"+password, nil
}

type loginLimiter struct{ keys map[string]int }

func (l *loginLimiter) Allow(_ context.Context, key string, limit int, _ time.Duration) (ratelimit.Result, error) {
	if l.keys == nil {
		l.keys = make(map[string]int)
	}
	l.keys[key]++
	return ratelimit.Result{Allowed: l.keys[key] <= limit, Limit: limit, RetryAfter: time.Minute}, nil
}

package integration_test

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	authapp "github.com/janickiy/go-recorder/internal/app/auth"
	conferencesapp "github.com/janickiy/go-recorder/internal/app/conferences"
	postgresinfra "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	authusecase "github.com/janickiy/go-recorder/internal/usecase/auth"
	conferenceusecase "github.com/janickiy/go-recorder/internal/usecase/conferences"
)

// TestFrontendPostgres проверяет сценарий «интерфейс Postgres», фиксируя ошибки поведения как регрессию.
// Внешняя команда или запрос использует контекст операции.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestFrontendPostgres(t *testing.T) {
	if os.Getenv("RECORDER_FRONTEND_E2E") != "true" {
		t.Skip("set RECORDER_FRONTEND_E2E=true and RECORDER_STAGE1_TEST_POSTGRES_DSN to enable")
	}
	db := stageOneDatabase(t)
	gin.SetMode(gin.TestMode)
	userRepo := postgresinfra.NewUserRepository(db)
	conferenceRepo := postgresinfra.NewConferenceRepository(db)
	tokens, err := security.NewTokenService(strings.Repeat("frontend-test-only-", 3))
	if err != nil {
		t.Fatal(err)
	}
	auth, err := authusecase.NewService(userRepo, security.PasswordHasher{}, tokens)
	if err != nil {
		t.Fatal(err)
	}
	conferences := conferenceusecase.NewService(conferenceRepo, userRepo, security.GenerateInviteCode)
	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	httptransport.RegisterPlatformRoutes(router, authapp.NewHandler(auth), conferencesapp.NewHandler(conferences), httpmiddleware.Authenticate(tokens))
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "npm", "run", "test:e2e", "--", "e2e/live.spec.ts", "--workers=1")
	command.Dir = "../../frontend"
	command.Env = append(os.Environ(), "API_PROXY_TARGET="+server.URL, "MEET_LIVE_TEST_URL=http://127.0.0.1:5175")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("frontend browser tests: %v\n%s", err, output)
	}
	t.Logf("%s", output)
}

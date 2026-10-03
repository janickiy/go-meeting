package integration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	authapp "github.com/janickiy/go-recorder/internal/app/auth"
	conferencesapp "github.com/janickiy/go-recorder/internal/app/conferences"
	"github.com/janickiy/go-recorder/internal/domain/users"
	postgresinfra "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	authusecase "github.com/janickiy/go-recorder/internal/usecase/auth"
)

func TestStageNineProfileUpdate(t *testing.T) {
	db := stageOneDatabase(t)
	userRepo := postgresinfra.NewUserRepository(db)
	tokens, err := security.NewTokenService(strings.Repeat("profile-integration-secret-", 3))
	if err != nil {
		t.Fatal(err)
	}
	authService, err := authusecase.NewService(userRepo, security.PasswordHasher{}, tokens)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	httptransport.RegisterPlatformRoutes(router, authapp.NewHandler(authService), conferencesapp.NewHandler(nil), httpmiddleware.Authenticate(tokens))
	api := stageOneAPI{router: router}
	first, firstToken := api.registerAndLogin(t, "first-profile@example.com", "First")
	second, secondToken := api.registerAndLogin(t, "second-profile@example.com", "Second")
	ctx := context.Background()
	before, err := userRepo.GetByID(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&users.User{}).Where("id = ?", first.ID).Update("is_admin", true).Error; err != nil {
		t.Fatal(err)
	}
	api.expect(t, "PATCH", "/auth/me", "", map[string]any{"displayName": "Unauthorized"}, 401, nil)
	api.expect(t, "PATCH", "/auth/me", firstToken, map[string]any{"displayName": "  "}, 422, nil)
	api.expect(t, "PATCH", "/auth/me", firstToken, map[string]any{"displayName": "Forged", "isAdmin": false}, 400, nil)
	api.expect(t, "PATCH", "/auth/me", firstToken, map[string]any{"displayName": "Forged", "userId": second.ID}, 400, nil)
	var response struct {
		Status string     `json:"status"`
		User   users.View `json:"user"`
	}
	api.expect(t, "PATCH", "/auth/me", firstToken, map[string]any{"displayName": "  Новое имя  "}, 200, &response)
	if response.Status != "success" || response.User.ID != first.ID || response.User.DisplayName == nil || *response.User.DisplayName != "Новое имя" || !response.User.IsAdmin || response.User.Email != first.Email {
		t.Fatal("profile response lost user identity, role or normalized name")
	}
	firstAfter, err := userRepo.GetByID(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondAfter, err := userRepo.GetByID(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if firstAfter.PasswordHash != before.PasswordHash || firstAfter.Email != before.Email || !firstAfter.IsAdmin || firstAfter.DisplayName == nil || *firstAfter.DisplayName != "Новое имя" {
		t.Fatal("profile update changed protected account fields")
	}
	if secondAfter.DisplayName == nil || *secondAfter.DisplayName != "Second" {
		t.Fatal("profile update changed another account")
	}
	var current struct {
		User users.View `json:"user"`
	}
	api.expect(t, "GET", "/auth/me", firstToken, nil, 200, &current)
	if current.User.DisplayName == nil || *current.User.DisplayName != "Новое имя" {
		t.Fatal("existing JWT did not read updated profile")
	}
	api.expect(t, "GET", "/auth/me", secondToken, nil, 200, &current)
	if current.User.DisplayName == nil || *current.User.DisplayName != "Second" {
		t.Fatal("other account profile was changed")
	}
}

package integration_test

import (
	"context"
	"github.com/gin-gonic/gin"
	authapp "github.com/janickiy/go-recorder/internal/app/auth"
	conferenceapp "github.com/janickiy/go-recorder/internal/app/conferences"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/users"
	postgres "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	transport "github.com/janickiy/go-recorder/internal/transport/http"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	authcase "github.com/janickiy/go-recorder/internal/usecase/auth"
	conferencecase "github.com/janickiy/go-recorder/internal/usecase/conferences"
	"strings"
	"testing"
)

func TestPublicGuestInvitation(t *testing.T) {
	db := stageOneDatabase(t)
	tokens, _ := security.NewTokenService(strings.Repeat("guest-integration-secret", 3))
	usersRepo := postgres.NewUserRepository(db)
	auth, _ := authcase.NewService(usersRepo, security.PasswordHasher{}, tokens)
	service := conferencecase.NewService(postgres.NewConferenceRepository(db), usersRepo, security.GenerateInviteCode)
	guest := &conferencecase.GuestService{Repository: postgres.NewGuestRepository(db), Tokens: tokens}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	transport.RegisterPlatformRoutes(router, authapp.NewHandler(auth), conferenceapp.NewHandler(service), middleware.Authenticate(tokens))
	transport.RegisterGuestRoutes(router, &conferenceapp.GuestHandler{Service: guest, Tokens: tokens}, nil)
	api := stageOneAPI{router: router}
	_, ownerToken := api.registerAndLogin(t, "guest-owner@example.test", "Owner")
	var created struct{ Item conferences.View }
	api.expect(t, "POST", "/conferences", ownerToken, map[string]any{"title": "Guest room", "waitingRoomEnabled": true}, 201, &created)
	room := created.Item
	preview := "/conference-invites/" + room.InviteCode
	var public struct{ Item map[string]any }
	api.expect(t, "GET", preview, "", nil, 200, &public)
	for _, key := range []string{"ownerId", "inviteCode", "inviteUrl", "participants"} {
		if _, exists := public.Item[key]; exists {
			t.Fatalf("public preview exposed %s", key)
		}
	}
	var count int64
	db.Model(&users.User{}).Count(&count)
	before := count
	api.expect(t, "POST", preview+"/guest", "", map[string]any{"displayName": "   "}, 422, nil)
	api.expect(t, "POST", preview+"/guest", "", map[string]any{"displayName": "Guest", "isAdmin": true}, 400, nil)
	api.expect(t, "POST", "/conference-invites/"+strings.Repeat("z", 32)+"/guest", "", map[string]any{"displayName": "Guest"}, 404, nil)
	db.Model(&users.User{}).Count(&count)
	if count != before {
		t.Fatal("failed join persisted a guest")
	}
	var session conferencecase.GuestSession
	api.expect(t, "POST", preview+"/guest", "", map[string]any{"displayName": "  Гость <script>  "}, 200, &session)
	token := session.AccessToken
	if session.User.GuestConferenceID == nil || *session.User.GuestConferenceID != room.ID || session.User.IsAdmin || session.User.Email != "" || session.Item.Role != conferences.ParticipantRole || session.Item.Status != conferences.Joined || session.Item.AdmissionState != conferences.AdmissionAdmitted {
		t.Fatalf("invalid guest session: %+v", session.Item)
	}
	userID, scope, _, err := tokens.VerifySession(token)
	if err != nil || userID != session.User.ID || scope != room.ID {
		t.Fatal("invalid scoped token")
	}
	api.expect(t, "GET", "/auth/me", token, nil, 200, nil)
	api.expect(t, "GET", "/me/conferences", token, nil, 403, nil)
	api.expect(t, "POST", "/conferences", token, map[string]any{"title": "Forbidden"}, 403, nil)
	api.expect(t, "PATCH", "/auth/me", token, map[string]any{"displayName": "Forbidden"}, 403, nil)
	api.expect(t, "GET", "/conferences/"+room.ID+"/participants", token, nil, 200, nil)
	// A guest waiting since the previous release is admitted on the same link.
	if err := db.Model(&conferences.Participant{}).Where("id = ?", session.Item.ID).Updates(map[string]any{"status": conferences.Waiting, "admission_state": conferences.AdmissionWaiting, "joined_at": nil}).Error; err != nil {
		t.Fatal(err)
	}
	var resumed conferencecase.GuestSession
	api.expect(t, "POST", preview+"/guest", token, map[string]any{"displayName": "Мария"}, 200, &resumed)
	if resumed.User.ID != session.User.ID || resumed.Item.ID != session.Item.ID || resumed.Item.DisplayName != "Мария" || resumed.Item.Status != conferences.Joined || resumed.Item.AdmissionState != conferences.AdmissionAdmitted {
		t.Fatal("guest resume replaced identity")
	}
	stored, err := usersRepo.GetByID(context.Background(), session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	api.expect(t, "POST", "/auth/login", "", map[string]any{"email": stored.Email, "password": stageOneTestPassword}, 401, nil)
	path := "/conferences/" + room.ID
	api.expect(t, "POST", path+"/join", ownerToken, nil, 200, nil)
	api.expect(t, "POST", path+"/start", ownerToken, nil, 200, nil)
	api.expect(t, "GET", path+"/participants", token, nil, 200, nil)
	api.expect(t, "POST", path+"/finish", token, nil, 403, nil)
	// Account holders follow the same immediate admission policy as guests.
	_, accountToken := api.registerAndLogin(t, "link-member@example.test", "Account")
	var account struct{ Item conferences.ParticipantView }
	api.expect(t, "POST", preview+"/join", accountToken, nil, 200, &account)
	if account.Item.Status != conferences.Joined || account.Item.AdmissionState != conferences.AdmissionAdmitted {
		t.Fatal("account link still waits")
	}
	if err := db.Model(&conferences.Participant{}).Where("id = ?", account.Item.ID).Updates(map[string]any{"status": conferences.Waiting, "admission_state": conferences.AdmissionWaiting, "joined_at": nil}).Error; err != nil {
		t.Fatal(err)
	}
	api.expect(t, "POST", preview+"/join", accountToken, nil, 200, &account)
	if account.Item.Status != conferences.Joined {
		t.Fatal("existing waiting account still waits")
	}

	api.expect(t, "POST", preview+"/guest", ownerToken, map[string]any{"displayName": "Wrong account"}, 403, nil)
	// A rejected guest cannot reset admission by reusing their session.
	if err := db.Model(&conferences.Participant{}).Where("id = ?", session.Item.ID).Updates(map[string]any{"status": conferences.Rejected, "admission_state": conferences.AdmissionRejected}).Error; err != nil {
		t.Fatal(err)
	}
	api.expect(t, "POST", preview+"/guest", token, map[string]any{"displayName": "Retry"}, 403, nil)
	api.expect(t, "POST", path+"/finish", ownerToken, nil, 200, nil)
	api.expect(t, "POST", preview+"/guest", "", map[string]any{"displayName": "Late"}, 409, nil)
}

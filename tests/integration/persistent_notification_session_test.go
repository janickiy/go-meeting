package integration_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	notificationsapp "github.com/janickiy/go-recorder/internal/app/notifications"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	authcase "github.com/janickiy/go-recorder/internal/usecase/auth"
)

type persistentNotificationFixture struct {
	stage   *stageTwoFixture
	auth    *authcase.Service
	login   users.LoginResponse
	bus     *redisinfra.NotificationBus
	handler *notificationsapp.Handler
	server  *httptest.Server
}

func persistentNotification(t *testing.T) *persistentNotificationFixture {
	t.Helper()
	stage := stageTwo(t)
	auth, err := authcase.NewService(pg.NewUserRepository(stage.db), security.PasswordHasher{}, stage.tokens)
	if err != nil {
		t.Fatal(err)
	}
	auth.WithSessions(pg.NewAuthSessionRepository(stage.db))
	login, err := auth.Login(context.Background(), users.LoginRequest{Email: stage.owner.Email, Password: stageOneTestPassword})
	if err != nil {
		t.Fatal(err)
	}
	f := &persistentNotificationFixture{stage: stage, auth: auth, login: login, bus: redisinfra.NewNotificationBus(stage.redis, stage.config.Namespace)}
	f.handler = notificationsapp.NewHandler(nil, f.bus, auth, nil, stage.config.Namespace)
	router := gin.New()
	httptransport.RegisterNotificationRoutes(router, f.handler, httpmiddleware.Authenticate(auth))
	f.server = httptest.NewServer(router)
	t.Cleanup(func() {
		f.handler.BeginDrain()
		f.server.Close()
	})
	return f
}

// Each reader owns a real HTTP body; channel completion proves that the server
// released the request rather than merely pausing notification publication.
type persistentNotificationStream struct {
	body   io.ReadCloser
	lines  chan string
	done   chan error
	cancel context.CancelFunc
}

func openPersistentNotificationStream(t *testing.T, server *httptest.Server, token string) *persistentNotificationStream {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/notifications/events", nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := server.Client().Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		response.Body.Close()
		cancel()
		t.Fatalf("persistent SSE refused: status=%d", response.StatusCode)
	}
	s := &persistentNotificationStream{body: response.Body, lines: make(chan string, 32), done: make(chan error, 1), cancel: cancel}
	go func() {
		scanner := bufio.NewScanner(s.body)
		for scanner.Scan() {
			s.lines <- scanner.Text()
		}
		close(s.lines)
		s.done <- scanner.Err()
	}()
	t.Cleanup(func() {
		cancel()
		response.Body.Close()
	})
	select {
	case line := <-s.lines:
		if line != ": connected" {
			t.Fatalf("missing initial SSE comment: %q", line)
		}
	case <-time.After(time.Second):
		t.Fatal("SSE connection did not flush its initial comment")
	}
	return s
}

func (s *persistentNotificationStream) expectClosed(t *testing.T) {
	t.Helper()
	select {
	case err := <-s.done:
		if err != nil {
			t.Fatalf("SSE did not close cleanly: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SSE request remained open")
	}
}

func (f *persistentNotificationFixture) waitSubscriptions(t *testing.T, want int64) {
	t.Helper()
	channel := f.stage.config.Namespace + ":notification:" + f.stage.owner.ID
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		counts, err := f.stage.redis.PubSubNumSub(context.Background(), channel).Result()
		if err != nil {
			t.Fatal(err)
		}
		if counts[channel] == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("notification subscriptions did not become %d", want)
}

func TestPersistentNotificationDrainClosesActiveStreamsPromptlyAndIdempotently(t *testing.T) {
	f := persistentNotification(t)
	streams := make([]*persistentNotificationStream, 4)
	for i := range streams {
		streams[i] = openPersistentNotificationStream(t, f.server, f.login.AccessToken)
	}
	f.waitSubscriptions(t, 4)
	begin := time.Now()
	var wait sync.WaitGroup
	for range 12 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			f.handler.BeginDrain()
		}()
	}
	wait.Wait()
	for _, stream := range streams {
		stream.expectClosed(t)
	}
	if time.Since(begin) > time.Second {
		t.Fatal("API drain waited for the one-hour JWT expiry or SSE keepalive")
	}
	f.waitSubscriptions(t, 0)
	if _, err := f.auth.Refresh(context.Background(), f.login.SessionToken); err != nil {
		t.Fatal("draining notification transport logged out its account", err)
	}
}

func TestPersistentNotificationRevokedSessionClosesBeforeNextEventDelivery(t *testing.T) {
	f := persistentNotification(t)
	stream := openPersistentNotificationStream(t, f.server, f.login.AccessToken)
	before := domain.Event("notification.created", "", map[string]string{"marker": "before logout"})
	if err := f.bus.Publish(context.Background(), f.stage.owner.ID, before); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	delivered := false
	for !delivered {
		select {
		case line, ok := <-stream.lines:
			if !ok {
				t.Fatal("valid persistent notification stream closed before logout")
			}
			delivered = strings.HasPrefix(line, "data: ") && strings.Contains(line, before.ID)
		case <-deadline.C:
			t.Fatal("real Redis notification was not delivered to the authorized stream")
		}
	}
	if err := f.auth.Logout(context.Background(), f.login.SessionToken, f.login.AccessToken); err != nil {
		t.Fatal(err)
	}
	after := domain.Event("notification.created", "", map[string]string{"marker": "private after logout"})
	if err := f.bus.Publish(context.Background(), f.stage.owner.ID, after); err != nil {
		t.Fatal(err)
	}
	stream.expectClosed(t)
	for line := range stream.lines {
		if strings.HasPrefix(line, "data: ") || strings.Contains(line, after.ID) {
			t.Fatalf("revoked SID received another private notification: %q", line)
		}
	}
	f.waitSubscriptions(t, 0)
	request, err := http.NewRequest(http.MethodGet, f.server.URL+"/api/v1/notifications/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+f.login.AccessToken)
	response, err := f.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked SID reopened a notification stream: status=%d", response.StatusCode)
	}
}

type unavailableNotificationVerifier struct {
	*authcase.Service
	calls  atomic.Int64
	failAt int64
}

func (v *unavailableNotificationVerifier) VerifyAuthorization(ctx context.Context, raw string) (string, string, string, time.Time, error) {
	if v.calls.Add(1) == v.failAt {
		return "", "", "", time.Time{}, apperrors.ErrUnavailable
	}
	return v.Service.VerifyAuthorization(ctx, raw)
}

func TestPersistentNotificationInitialStoreOutageReturns503InsteadOf401(t *testing.T) {
	f := persistentNotification(t)
	for _, failAt := range []int64{1, 2} {
		t.Run(fmt.Sprintf("authorization_check_%d", failAt), func(t *testing.T) {
			verifier := &unavailableNotificationVerifier{Service: f.auth, failAt: failAt}
			handler := notificationsapp.NewHandler(nil, f.bus, verifier, nil, f.stage.config.Namespace)
			router := gin.New()
			httptransport.RegisterNotificationRoutes(router, handler, httpmiddleware.Authenticate(verifier))
			server := httptest.NewServer(router)
			defer server.Close()
			req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/notifications/events", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+f.login.AccessToken)
			response, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusServiceUnavailable || strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
				t.Fatalf("temporary lookup failure invalidated authentication: status=%d", response.StatusCode)
			}
			if _, err := f.auth.Refresh(context.Background(), f.login.SessionToken); err != nil {
				t.Fatal("temporary notification auth failure revoked the account", err)
			}
			f.waitSubscriptions(t, 0)
		})
	}
}

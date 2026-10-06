package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	chatapp "github.com/janickiy/go-recorder/internal/app/chat"
	personalapp "github.com/janickiy/go-recorder/internal/app/personal"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	s3storage "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	chatusecase "github.com/janickiy/go-recorder/internal/usecase/chat"
	personalusecase "github.com/janickiy/go-recorder/internal/usecase/personal"
)

type groupAssetFixture struct {
	base       *stageTwoFixture
	repo       *pg.PersonalRepository
	assetsRepo *pg.PersonalAssetRepository
	assets     *personalusecase.AssetService
	storage    *s3storage.Client
	chat       *chatusecase.Service
	group      personal.Conversation
	router     *gin.Engine
}

func groupAssets(t *testing.T) *groupAssetFixture {
	t.Helper()
	if os.Getenv("RECORDER_DIRECT_E2E") != "true" {
		t.Skip("RECORDER_DIRECT_E2E=true enables private group asset integration")
	}
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewPersonalRepository(f.db)
	group, _, err := repo.CreateGroup(ctx, f.owner.ID, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Private assets", MemberIDs: []string{f.member.ID}})
	if err != nil {
		t.Fatal(err)
	}
	storage, err := s3storage.NewClient(ctx, os.Getenv("RECORDER_STAGE4_TEST_MINIO_ENDPOINT"), os.Getenv("RECORDER_STAGE4_TEST_MINIO_ACCESS_KEY"), os.Getenv("RECORDER_STAGE4_TEST_MINIO_SECRET_KEY"), "recordings", false)
	if err != nil {
		t.Fatal(err)
	}
	assetsRepo := pg.NewPersonalAssetRepository(f.db)
	assets, err := personalusecase.NewAssetService(ctx, assetsRepo, storage, repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	chatService, err := chatusecase.NewService(ctx, pg.NewDirectChatRepository(f.db), storage, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := &personalapp.Handler{Repo: repo}
	router := gin.New()
	httptransport.RegisterPersonalRoutes(router, h, chatapp.NewHandler(chatService).ForConversations().WithGroupDownloads(assets), middleware.Authenticate(f.tokens), nil)
	httptransport.RegisterPersonalAssetRoutes(router, personalapp.NewAssetHandler(assets), middleware.Authenticate(f.tokens), h.AccountOnly, nil)
	t.Cleanup(func() {
		_ = storage.RemovePrefix(ctx, "avatars/conversations/"+group.ID+"/")
		_ = storage.RemovePrefix(ctx, "attachments/direct/"+group.ID+"/")
	})
	return &groupAssetFixture{f, repo, assetsRepo, assets, storage, chatService, group, router}
}
func groupPNG(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func (f *groupAssetFixture) request(t *testing.T, method, path, token, contentType string, body []byte, want int) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	if response.Code != want {
		t.Fatalf("%s %s=%d want%d: %s", method, path, response.Code, want, response.Body.String())
	}
	return response
}
func TestGroupProtectedAssetsAuthorizationAndCleanup(t *testing.T) {
	f := groupAssets(t)
	ctx := context.Background()
	path := "/conversations/" + f.group.ID
	outsider := users.User{ID: uuid.NewString(), Email: "outside-assets@example.test", PasswordHash: "unused"}
	if _, err := pg.NewUserRepository(f.base.db).Create(ctx, outsider); err != nil {
		t.Fatal(err)
	}
	outsideToken, _ := f.base.tokens.Issue(outsider.ID)
	guest := users.User{ID: uuid.NewString(), Email: "guest-assets@example.test", PasswordHash: "unused", GuestConferenceID: &f.base.conference.ID}
	if _, err := pg.NewUserRepository(f.base.db).Create(ctx, guest); err != nil {
		t.Fatal(err)
	}
	guestToken, _ := f.base.tokens.IssueGuest(guest.ID, f.base.conference.ID)
	data := groupPNG(t)
	f.request(t, "PUT", path+"/avatar", f.base.memberToken, "image/png", data, 403)
	f.request(t, "PUT", path+"/avatar", outsideToken, "image/png", data, 403)
	f.request(t, "PUT", path+"/avatar", guestToken, "image/png", data, 403)
	f.request(t, "PUT", path+"/avatar", f.base.ownerToken, "image/svg+xml", []byte("<svg/>"), 422)
	f.request(t, "PUT", path+"/avatar", f.base.ownerToken, "image/png", make([]byte, personalusecase.MaxAvatarBytes+1), 422)
	response := f.request(t, "PUT", path+"/avatar", f.base.ownerToken, "image/png", data, 200)
	if strings.Contains(response.Body.String(), "avatars/") || strings.Contains(response.Body.String(), "avatar_key") {
		t.Fatal("server storage key leaked")
	}
	var first struct{ Item personal.Conversation }
	if err := json.Unmarshal(response.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.Item.AvatarVersion == nil {
		t.Fatal("missing public avatar version")
	}
	read := f.request(t, "GET", path+"/avatar/content", f.base.memberToken, "", nil, 200)
	if read.Header().Get("Cache-Control") != "private, no-store" || read.Header().Get("Content-Type") != "image/png" {
		t.Fatal("private image response headers missing")
	}
	if _, err := png.Decode(read.Body); err != nil {
		t.Fatal(err)
	}
	f.request(t, "GET", path+"/avatar/content", outsideToken, "", nil, 403)
	f.request(t, "GET", path+"/avatar/content?token="+f.base.memberToken, "", "", nil, 401)
	f.request(t, "DELETE", path+"/avatar", f.base.memberToken, "", nil, 403)
	f.request(t, "PUT", path+"/avatar", f.base.ownerToken, "image/png", data, 200)
	if err := f.assets.CleanAvatars(ctx); err != nil {
		t.Fatal(err)
	}
	oldKey := "avatars/conversations/" + f.group.ID + "/" + *first.Item.AvatarVersion
	if body, _, _, err := f.storage.OpenPersonalAsset(ctx, oldKey); err == nil {
		body.Close()
		t.Fatal("replaced object retained")
	}
	f.request(t, "GET", path+"/avatar/content", f.base.memberToken, "", nil, 200)
	// Simulate a process crash after PutObject but before the activation commit.
	version := uuid.NewString()
	sum := sha256.Sum256(data)
	orphan := personalusecase.Asset{Version: version, ConversationID: f.group.ID, Key: "avatars/conversations/" + f.group.ID + "/" + version, ContentType: "image/png", Size: int64(len(data)), Checksum: hex.EncodeToString(sum[:])}
	if err := f.assetsRepo.BeginAvatar(ctx, f.base.owner.ID, f.group.ID, orphan); err != nil {
		t.Fatal(err)
	}
	if err := f.storage.PutPersonalAvatar(ctx, orphan.Key, bytes.NewReader(data), orphan.Size, orphan.ContentType, orphan.Checksum); err != nil {
		t.Fatal(err)
	}
	if err := f.base.db.Exec("UPDATE conversation_avatar_objects SET cleanup_after=clock_timestamp()-INTERVAL '1 second' WHERE id=?", version).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.assets.CleanAvatars(ctx); err != nil {
		t.Fatal(err)
	}
	if body, _, _, err := f.storage.OpenPersonalAsset(ctx, orphan.Key); err == nil {
		body.Close()
		t.Fatal("interrupted upload not cleaned")
	}
	if _, err := f.assetsRepo.ActivateAvatar(ctx, f.base.owner.ID, f.group.ID, version); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("cleaned object became active: %v", err)
	}
	f.request(t, "GET", path+"/avatar/content", f.base.memberToken, "", nil, 200)
	if _, _, err := f.repo.RemoveGroupMember(ctx, f.base.owner.ID, f.group.ID, f.base.member.ID); err != nil {
		t.Fatal(err)
	}
	f.request(t, "GET", path+"/avatar/content", f.base.memberToken, "", nil, 403)
	f.request(t, "DELETE", path+"/avatar", f.base.ownerToken, "", nil, 200)
	if err := f.assets.CleanAvatars(ctx); err != nil {
		t.Fatal(err)
	}
	f.request(t, "GET", path+"/avatar/content", f.base.ownerToken, "", nil, 404)
}

func TestGroupAttachmentBytesRevocationAndDirectFallback(t *testing.T) {
	f := groupAssets(t)
	ctx := context.Background()
	path := "/conversations/" + f.group.ID
	data := []byte("private group attachment\n")
	attachment, _, err := f.chat.InitAttachment(ctx, f.base.owner.ID, f.group.ID, chat.InitRequest{ClientRequestID: uuid.NewString(), Filename: "private.txt", MimeType: "text/plain", Size: int64(len(data))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.chat.Upload(ctx, f.base.owner.ID, f.group.ID, attachment.ID, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.chat.FinalizeAttachment(ctx, f.base.owner.ID, f.group.ID, attachment.ID); err != nil {
		t.Fatal(err)
	}
	f.request(t, "GET", path+"/attachments/"+attachment.ID+"/content", f.base.ownerToken, "", nil, 404)
	if _, _, err = f.chat.Send(ctx, f.base.owner.ID, f.group.ID, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "attached", AttachmentIDs: []string{attachment.ID}}); err != nil {
		t.Fatal(err)
	}
	response := f.request(t, "GET", path+"/attachments/"+attachment.ID+"/download", f.base.memberToken, "", nil, 200)
	var result struct {
		URL           string
		Authenticated bool
	}
	if err = json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Authenticated || result.URL != "/api/v1"+path+"/attachments/"+attachment.ID+"/content" || strings.Contains(result.URL, "X-Amz") {
		t.Fatalf("group signed URL escaped: %s", response.Body.String())
	}
	content := f.request(t, "GET", path+"/attachments/"+attachment.ID+"/content", f.base.memberToken, "", nil, 200)
	if !bytes.Equal(content.Body.Bytes(), data) || content.Header().Get("Content-Type") != "application/octet-stream" || content.Header().Get("Content-Disposition") == "" {
		t.Fatal("protected file response changed")
	}
	other, _, err := f.repo.CreateGroup(ctx, f.base.owner.ID, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	f.request(t, "GET", "/conversations/"+other.ID+"/attachments/"+attachment.ID+"/content", f.base.ownerToken, "", nil, 404)
	if _, _, err = f.repo.RemoveGroupMember(ctx, f.base.owner.ID, f.group.ID, f.base.member.ID); err != nil {
		t.Fatal(err)
	}
	f.request(t, "GET", path+"/attachments/"+attachment.ID+"/content", f.base.memberToken, "", nil, 403)
	f.request(t, "GET", path+"/attachments/"+attachment.ID+"/download", f.base.memberToken, "", nil, 403)
	direct, _, err := f.repo.GetOrCreate(ctx, f.base.owner.ID, f.base.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	handled, _, _, err := f.assets.GroupDownload(ctx, f.base.owner.ID, direct.ID, attachment.ID)
	if handled || err != nil {
		t.Fatalf("direct signed flow intercepted: %v %v", handled, err)
	}
	f.request(t, "GET", "/conversations/"+direct.ID+"/attachments/"+attachment.ID+"/content", f.base.ownerToken, "", nil, 404)
}

func TestGroupAvatarStreamSerializesMembershipRemoval(t *testing.T) {
	f := groupAssets(t)
	ctx := context.Background()
	if _, err := f.assets.PutAvatar(ctx, f.base.owner.ID, f.group.ID, "image/png", bytes.NewReader(groupPNG(t))); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	streamDone := make(chan error, 1)
	removeDone := make(chan error, 1)
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	go func() {
		streamDone <- f.assets.Stream(ctx, f.base.member.ID, f.group.ID, "", func(_ personalusecase.Asset, reader io.Reader) error {
			close(entered)
			<-release
			_, err := io.Copy(io.Discard, reader)
			return err
		})
	}()
	select {
	case <-entered:
	case err := <-streamDone:
		t.Fatalf("stream not authorized: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("stream start timeout")
	}
	go func() {
		_, _, err := f.repo.RemoveGroupMember(ctx, f.base.owner.ID, f.group.ID, f.base.member.ID)
		removeDone <- err
	}()
	select {
	case err := <-removeDone:
		t.Fatalf("removal committed while private bytes still in-flight: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	once.Do(func() { close(release) })
	select {
	case err := <-streamDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream release timeout")
	}
	select {
	case err := <-removeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("removal release timeout")
	}
	if err := f.assets.Stream(ctx, f.base.member.ID, f.group.ID, "", func(_ personalusecase.Asset, _ io.Reader) error { t.Fatal("removed member received bytes"); return nil }); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("new read after removal: %v", err)
	}
}

type blockedGroupAvatarStorage struct {
	personalusecase.AssetStorage
	uploaded, proceed chan struct{}
}

func (s *blockedGroupAvatarStorage) PutPersonalAvatar(ctx context.Context, key string, reader io.Reader, size int64, typ, checksum string) error {
	if err := s.AssetStorage.PutPersonalAvatar(ctx, key, reader, size, typ, checksum); err != nil {
		return err
	}
	close(s.uploaded)
	select {
	case <-s.proceed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestGroupAdminAvatarUploadReauthorizesAfterMembershipRemoval(t *testing.T) {
	f := groupAssets(t)
	ctx := context.Background()
	if _, err := f.repo.ChangeGroupRole(ctx, f.base.owner.ID, f.group.ID, f.base.member.ID, personal.Admin); err != nil {
		t.Fatal(err)
	}
	// A normal admin upload is allowed, and admins can clear their group's avatar.
	if _, err := f.assets.PutAvatar(ctx, f.base.member.ID, f.group.ID, "image/png", bytes.NewReader(groupPNG(t))); err != nil {
		t.Fatal(err)
	}
	if _, err := f.assets.DeleteAvatar(ctx, f.base.member.ID, f.group.ID); err != nil {
		t.Fatal(err)
	}
	storage := &blockedGroupAvatarStorage{AssetStorage: f.storage, uploaded: make(chan struct{}), proceed: make(chan struct{})}
	service, err := personalusecase.NewAssetService(ctx, f.assetsRepo, storage, f.repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(storage.proceed) }) })
	data := groupPNG(t)
	go func() {
		_, err := service.PutAvatar(ctx, f.base.member.ID, f.group.ID, "image/png", bytes.NewReader(data))
		result <- err
	}()
	select {
	case <-storage.uploaded:
	case err := <-result:
		t.Fatalf("admin upload failed early: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("upload timeout")
	}
	if _, _, err = f.repo.RemoveGroupMember(ctx, f.base.owner.ID, f.group.ID, f.base.member.ID); err != nil {
		t.Fatal(err)
	}
	once.Do(func() { close(storage.proceed) })
	select {
	case err = <-result:
		if !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatalf("removed admin activated upload: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("activation timeout")
	}
	item, err := f.repo.Get(ctx, f.base.owner.ID, f.group.ID)
	if err != nil || item.AvatarVersion != nil {
		t.Fatalf("removed admin changed avatar: %v %v", item.AvatarVersion, err)
	}
	if err = f.base.db.Exec("UPDATE conversation_avatar_objects SET cleanup_after=clock_timestamp()-INTERVAL '1 second' WHERE conversation_id=? AND state='pending'", f.group.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.assets.CleanAvatars(ctx); err != nil {
		t.Fatal(err)
	}
	var remaining int64
	if err = f.base.db.Table("conversation_avatar_objects").Where("conversation_id=? AND state<>'cleaned'", f.group.ID).Count(&remaining).Error; err != nil || remaining != 0 {
		t.Fatalf("removed upload not reconciled: %d %v", remaining, err)
	}
	// Soft deletion cleans the previously current private avatar as well.
	if _, err = f.assets.PutAvatar(ctx, f.base.owner.ID, f.group.ID, "image/png", bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.repo.DeleteGroup(ctx, f.base.owner.ID, f.group.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.assets.CleanAvatars(ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.base.db.Table("conversation_avatar_objects").Where("conversation_id=? AND state<>'cleaned'", f.group.ID).Count(&remaining).Error; err != nil || remaining != 0 {
		t.Fatalf("deleted group retained avatar: %d %v", remaining, err)
	}
}

func TestGroupAvatarCleanedMetadataRetentionIsBounded(t *testing.T) {
	f := groupAssets(t)
	ctx := context.Background()
	data := groupPNG(t)
	current, err := f.assets.PutAvatar(ctx, f.base.owner.ID, f.group.ID, "image/png", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if current.AvatarVersion == nil {
		t.Fatal("missing current avatar")
	}
	sum := sha256.Sum256(data)
	insert := func(state string, ageDays int) string {
		id := uuid.NewString()
		key := "avatars/conversations/" + f.group.ID + "/" + id
		if err := f.base.db.Exec(`INSERT INTO conversation_avatar_objects(id,conversation_id,object_key,content_type,size,checksum,state,updated_at)
		 VALUES(?,?,?,'image/png',?,?,?,clock_timestamp()-(?*INTERVAL '1 day'))`, id, f.group.ID, key, len(data), hex.EncodeToString(sum[:]), state, ageDays).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	old := []string{}
	for i := 0; i < 30; i++ {
		old = append(old, insert("cleaned", 8))
	}
	young := insert("cleaned", 6)
	pending := insert("pending", 8)
	// Defense in depth: even malformed cleaned metadata referenced by a current
	// pointer is retained; the normal current state is active.
	if err := f.base.db.Exec("UPDATE conversation_avatar_objects SET state='cleaned',updated_at=clock_timestamp()-INTERVAL '8 days' WHERE id=?", *current.AvatarVersion).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.assetsRepo.AvatarCleanupCandidates(ctx, 25); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = f.base.db.Table("conversation_avatar_objects").Where("id IN ?", old).Count(&count).Error; err != nil || count != 5 {
		t.Fatalf("purge was not limited to25: %d %v", count, err)
	}
	if err = f.base.db.Table("conversation_avatar_objects").Where("id IN ?", []string{young, pending, *current.AvatarVersion}).Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("protected lifecycle/current rows purged: %d %v", count, err)
	}
	if _, err = f.assetsRepo.AvatarCleanupCandidates(ctx, 25); err != nil {
		t.Fatal(err)
	}
	if err = f.base.db.Table("conversation_avatar_objects").Where("id IN ?", old).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("old cleaned rows retained: %d %v", count, err)
	}
}

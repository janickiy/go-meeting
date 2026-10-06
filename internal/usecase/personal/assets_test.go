package personal

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/personal"
)

func avatarPNG(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

type assetVisitRepo struct {
	AssetRepository
	visit func(context.Context, func(Asset) error) error
}

func (r assetVisitRepo) VisitAsset(ctx context.Context, _, _, _ string, visit func(Asset) error) error {
	return r.visit(ctx, visit)
}

type assetReaderStub struct{ ConversationReader }
type countedAssetBody struct{ closes int }

func (b *countedAssetBody) Read(p []byte) (int, error) { return 0, io.EOF }
func (b *countedAssetBody) Close() error               { b.closes++; return nil }

type assetStorageStub struct {
	AssetStorage
	body *countedAssetBody
}

func (s assetStorageStub) CheckAttachmentPrivacy(context.Context) error { return nil }
func (s assetStorageStub) OpenPersonalAsset(context.Context, string) (io.ReadCloser, int64, string, error) {
	return s.body, 1, "image/png", nil
}

func TestAvatarStreamCancellationClosesObjectAndReleasesSlot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	body := &countedAssetBody{}
	repo := assetVisitRepo{visit: func(ctx context.Context, visit func(Asset) error) error {
		return visit(Asset{Key: "private", Size: 1, ContentType: "image/png"})
	}}
	service, err := NewAssetService(ctx, repo, assetStorageStub{body: body}, assetReaderStub{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = service.Stream(ctx, uuid.NewString(), uuid.NewString(), "", func(_ Asset, _ io.Reader) error {
		cancel()
		return ctx.Err()
	})
	if err != context.Canceled || body.closes != 1 || len(service.streams) != 0 {
		t.Fatalf("stream leaked resources: err=%v closes=%d slots=%d", err, body.closes, len(service.streams))
	}
	if err = service.Stream(ctx, uuid.NewString(), uuid.NewString(), "", func(_ Asset, _ io.Reader) error { t.Fatal("canceled stream entered repository"); return nil }); err != context.Canceled {
		t.Fatalf("canceled stream: %v", err)
	}
}
func TestAvatarCanonicalValidation(t *testing.T) {
	valid := avatarPNG(t)
	polyglot := append(append([]byte{}, valid...), []byte("<script>alert('x')</script>")...)
	canonical, typ, err := normalizeAvatar(bytes.NewReader(polyglot), "image/png")
	if err != nil || typ != "image/png" || bytes.Contains(canonical, []byte("<script>")) {
		t.Fatalf("canonical encoding failed: %s %v", typ, err)
	}
	if _, err = png.Decode(bytes.NewReader(canonical)); err != nil {
		t.Fatal(err)
	}
	huge := append([]byte{}, valid...)
	binary.BigEndian.PutUint32(huge[16:20], 2049)
	binary.BigEndian.PutUint32(huge[29:33], crc32.ChecksumIEEE(huge[12:29]))
	for _, test := range []struct {
		name, mime string
		data       []byte
	}{
		{"mismatch", "image/jpeg", valid}, {"SVG", "image/svg+xml", []byte("<svg/>")},
		{"truncated", "image/png", valid[:35]}, {"huge dimensions", "image/png", huge},
		{"oversized", "image/png", make([]byte, MaxAvatarBytes+1)}, {"empty", "image/png", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := normalizeAvatar(bytes.NewReader(test.data), test.mime); err == nil {
				t.Fatal("unsafe image accepted")
			}
		})
	}
}

type assetMutationRepo struct {
	AssetRepository
	item      domain.Conversation
	err       error
	changed   bool
	committed bool
}

func (r *assetMutationRepo) BeginAvatar(context.Context, string, string, Asset) error { return nil }
func (r *assetMutationRepo) ActivateAvatar(_ context.Context, _, _, version string) (domain.Conversation, error) {
	r.item.AvatarVersion = &version
	r.committed = r.err == nil
	return r.item, r.err
}
func (r *assetMutationRepo) ClearAvatar(context.Context, string, string) (domain.Conversation, bool, error) {
	r.committed = r.err == nil
	return r.item, r.changed, r.err
}

type deniedAssetReader struct{ calls int }

func (r *deniedAssetReader) Get(context.Context, string, string) (domain.Conversation, error) {
	r.calls++
	return domain.Conversation{}, apperrors.ErrForbidden
}

type assetMutationStorage struct{ AssetStorage }

func (assetMutationStorage) CheckAttachmentPrivacy(context.Context) error { return nil }
func (assetMutationStorage) PutPersonalAvatar(context.Context, string, io.Reader, int64, string, string) error {
	return nil
}

type assetMutationEvents struct {
	t     *testing.T
	repo  *assetMutationRepo
	calls int
}

func (e *assetMutationEvents) PublishConversation(_ context.Context, id, kind string, _ any) error {
	if !e.repo.committed || id != e.repo.item.ID || kind != "conversation.updated" {
		e.t.Fatal("avatar event published before committed mutation")
	}
	e.calls++
	return nil
}

func TestAvatarMutationReturnsCommittedSnapshotWithoutReauthorization(t *testing.T) {
	for _, test := range []struct {
		name       string
		put        bool
		changed    bool
		err        error
		wantEvents int
	}{
		{"activate", true, true, nil, 1},
		{"activate failed", true, false, apperrors.ErrUnavailable, 0},
		{"clear", false, true, nil, 1},
		{"clear idempotent", false, false, nil, 0},
		{"clear failed", false, false, apperrors.ErrUnavailable, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			id := uuid.NewString()
			repo := &assetMutationRepo{item: domain.Conversation{ID: id, Type: "group", Name: "Committed snapshot", MyRole: domain.Admin}, err: test.err, changed: test.changed}
			reader := &deniedAssetReader{}
			events := &assetMutationEvents{t: t, repo: repo}
			service, err := NewAssetService(context.Background(), repo, assetMutationStorage{}, reader, events)
			if err != nil {
				t.Fatal(err)
			}
			var item domain.Conversation
			if test.put {
				item, err = service.PutAvatar(context.Background(), uuid.NewString(), id, "image/png", bytes.NewReader(avatarPNG(t)))
			} else {
				item, err = service.DeleteAvatar(context.Background(), uuid.NewString(), id)
			}
			if err != test.err || reader.calls != 0 || events.calls != test.wantEvents {
				t.Fatalf("post-commit contract changed: err=%v reads=%d events=%d", err, reader.calls, events.calls)
			}
			if err == nil && (item.ID != id || item.Name != repo.item.Name || item.MyRole != domain.Admin || test.put != (item.AvatarVersion != nil)) {
				t.Fatalf("committed snapshot was not returned: %+v", item)
			}
		})
	}
}

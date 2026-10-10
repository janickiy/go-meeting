package personal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	domain "github.com/janickiy/go-recorder/internal/domain/personal"
)

const MaxAvatarBytes int64 = 2 << 20
const AssetStreamTimeout = 30 * time.Second

// Asset keys are server-only data and never part of a public DTO.
type Asset struct {
	Version, ConversationID, Key, ContentType, Filename, Checksum string
	Size                                                          int64
	CleanupToken                                                  string
}

type AssetRepository interface {
	BeginAvatar(context.Context, string, string, Asset) error
	ActivateAvatar(context.Context, string, string, string) (domain.Conversation, error)
	ClearAvatar(context.Context, string, string) (domain.Conversation, bool, error)
	GroupAttachment(context.Context, string, string, string) (bool, Asset, error)
	// The callback runs under a conversation share lock. Membership mutations
	// lock that conversation exclusively, so removal cannot commit mid-stream.
	VisitAsset(context.Context, string, string, string, func(Asset) error) error
	AvatarCleanupCandidates(context.Context, int) ([]string, error)
	ClaimAvatarCleanup(context.Context, string) (*Asset, error)
	FinishAvatarCleanup(context.Context, Asset, bool) error
}
type AssetStorage interface {
	CheckAttachmentPrivacy(context.Context) error
	PutPersonalAvatar(context.Context, string, io.Reader, int64, string, string) error
	OpenPersonalAsset(context.Context, string) (io.ReadCloser, int64, string, error)
	DeletePersonalAvatar(context.Context, string) error
}
type AssetEvents interface {
	PublishConversation(context.Context, string, string, any) error
}
type AssetService struct {
	repo             AssetRepository
	storage          AssetStorage
	events           AssetEvents
	uploads, streams chan struct{}
}

func NewAssetService(ctx context.Context, repo AssetRepository, storage AssetStorage, events AssetEvents) (*AssetService, error) {
	if repo == nil || storage == nil {
		return nil, fmt.Errorf("personal assets dependencies are required")
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := storage.CheckAttachmentPrivacy(check); err != nil {
		return nil, err
	}
	return &AssetService{repo: repo, storage: storage, events: events, uploads: make(chan struct{}, 2), streams: make(chan struct{}, 8)}, nil
}

func assetIDs(user, conversation string) error {
	if _, err := chat.UUID(user); err != nil {
		return err
	}
	_, err := chat.UUID(conversation)
	return err
}
func acquireAssetSlot(ctx context.Context, slots chan struct{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case slots <- struct{}{}:
		return nil
	default:
		return apperrors.ErrRateLimited
	}
}

func (s *AssetService) PutAvatar(ctx context.Context, user, conversation, contentType string, body io.Reader) (domain.Conversation, error) {
	if err := assetIDs(user, conversation); err != nil {
		return domain.Conversation{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, AssetStreamTimeout)
	defer cancel()
	if err := acquireAssetSlot(ctx, s.uploads); err != nil {
		return domain.Conversation{}, err
	}
	defer func() { <-s.uploads }()
	data, typ, err := normalizeAvatar(body, contentType)
	if err != nil {
		return domain.Conversation{}, err
	}
	version := uuid.NewString()
	sum := sha256.Sum256(data)
	asset := Asset{Version: version, ConversationID: conversation, Key: "avatars/conversations/" + conversation + "/" + version, ContentType: typ, Size: int64(len(data)), Checksum: hex.EncodeToString(sum[:])}
	// Persist before uploading: crashes or an ambiguous object-store response
	// leave a durable pending row that the cleanup worker can recover.
	if err = s.repo.BeginAvatar(ctx, user, conversation, asset); err != nil {
		return domain.Conversation{}, err
	}
	if err = s.storage.PutPersonalAvatar(ctx, asset.Key, bytes.NewReader(data), asset.Size, typ, asset.Checksum); err != nil {
		return domain.Conversation{}, apperrors.Wrap(apperrors.ErrUnavailable, err, "avatar upload failed")
	}
	item, err := s.repo.ActivateAvatar(ctx, user, conversation, version)
	if err != nil {
		return domain.Conversation{}, err
	}
	s.changed(ctx, conversation)
	return item, nil
}
func (s *AssetService) DeleteAvatar(ctx context.Context, user, conversation string) (domain.Conversation, error) {
	if err := assetIDs(user, conversation); err != nil {
		return domain.Conversation{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, AssetStreamTimeout)
	defer cancel()
	item, changed, err := s.repo.ClearAvatar(ctx, user, conversation)
	if err != nil {
		return domain.Conversation{}, err
	}
	if changed {
		s.changed(ctx, conversation)
	}
	return item, nil
}
func (s *AssetService) changed(ctx context.Context, conversation string) {
	if s.events != nil {
		_ = s.events.PublishConversation(ctx, conversation, "conversation.updated", map[string]any{"conversationId": conversation, "type": "group"})
	}
}

func normalizeAvatar(body io.Reader, contentType string) ([]byte, string, error) {
	typ := strings.TrimSpace(strings.Split(contentType, ";")[0])
	if typ != "image/jpeg" && typ != "image/png" {
		return nil, "", apperrors.New(apperrors.ErrInvalidInput, "avatar must be JPEG or PNG")
	}
	raw, err := io.ReadAll(io.LimitReader(body, MaxAvatarBytes+1))
	if err != nil {
		return nil, "", apperrors.ErrInvalidInput
	}
	if len(raw) == 0 || int64(len(raw)) > MaxAvatarBytes {
		return nil, "", apperrors.New(apperrors.ErrInvalidInput, "avatar exceeds 2 MiB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "jpeg" && format != "png") || "image/"+format != typ || config.Width < 1 || config.Height < 1 || config.Width > 2048 || config.Height > 2048 || int64(config.Width)*int64(config.Height) > 4<<20 {
		return nil, "", apperrors.New(apperrors.ErrInvalidInput, "invalid avatar image or dimensions")
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", apperrors.ErrInvalidInput
	}
	// Canonical re-encoding strips embedded metadata and trailing/polyglot data.
	var out avatarBuffer
	if format == "jpeg" {
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&out, img)
	}
	if err != nil || int64(out.Len()) > MaxAvatarBytes {
		return nil, "", apperrors.New(apperrors.ErrInvalidInput, "encoded avatar exceeds 2 MiB")
	}
	return out.Bytes(), typ, nil
}

// Encoding may expand a small compressed input. Bound its output allocation as
// well as the input and decoded dimensions.
type avatarBuffer struct{ bytes.Buffer }

func (b *avatarBuffer) Write(data []byte) (int, error) {
	if int64(b.Len()+len(data)) > MaxAvatarBytes {
		return 0, apperrors.ErrInvalidInput
	}
	return b.Buffer.Write(data)
}

// GroupDownload returns a non-capability URL; each byte request reauthorizes.
func (s *AssetService) GroupDownload(ctx context.Context, user, conversation, attachment string) (bool, string, time.Time, error) {
	if err := assetIDs(user, conversation); err != nil {
		return true, "", time.Time{}, err
	}
	if _, err := chat.UUID(attachment); err != nil {
		return true, "", time.Time{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	handled, _, err := s.repo.GroupAttachment(ctx, user, conversation, attachment)
	if !handled || err != nil {
		return handled, "", time.Time{}, err
	}
	return true, "/api/v1/conversations/" + conversation + "/attachments/" + attachment + "/content", time.Now().UTC().Add(5 * time.Minute), nil
}

// Stream holds fresh membership authorization and the repository's share lock
// until the object reader closes. An empty attachment selects the current avatar.
func (s *AssetService) Stream(ctx context.Context, user, conversation, attachment string, deliver func(Asset, io.Reader) error) error {
	if err := assetIDs(user, conversation); err != nil {
		return err
	}
	if attachment != "" {
		if _, err := chat.UUID(attachment); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, AssetStreamTimeout)
	defer cancel()
	if err := acquireAssetSlot(ctx, s.streams); err != nil {
		return err
	}
	defer func() { <-s.streams }()
	return s.repo.VisitAsset(ctx, user, conversation, attachment, func(asset Asset) error {
		body, size, typ, err := s.storage.OpenPersonalAsset(ctx, asset.Key)
		if err != nil {
			return apperrors.Wrap(apperrors.ErrUnavailable, err, "asset is unavailable")
		}
		defer body.Close()
		if size != asset.Size || size < 1 || (attachment == "" && typ != asset.ContentType) {
			return apperrors.ErrUnavailable
		}
		return deliver(asset, io.LimitReader(body, asset.Size))
	})
}

func (s *AssetService) CleanAvatars(ctx context.Context) error {
	ids, err := s.repo.AvatarCleanupCandidates(ctx, 25)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		asset, err := s.repo.ClaimAvatarCleanup(ctx, id)
		if err != nil {
			return err
		}
		if asset == nil {
			continue
		}
		operation, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = s.storage.DeletePersonalAvatar(operation, asset.Key)
		cancel()
		if finishErr := s.repo.FinishAvatarCleanup(ctx, *asset, err == nil); finishErr != nil {
			return finishErr
		}
		if err != nil {
			return err
		}
	}
	return nil
}
func (s *AssetService) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		operation, cancel := context.WithTimeout(ctx, 20*time.Second)
		_ = s.CleanAvatars(operation)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

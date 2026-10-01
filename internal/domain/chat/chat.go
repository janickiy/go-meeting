package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

const (
	MaxTextRunes             = 4000
	MaxAttachments           = 5
	MaxAttachmentBytes int64 = 10 << 20
	AttachmentTTL            = 30 * time.Minute
	UploadLease              = 2 * time.Minute
	DownloadTTL              = 5 * time.Minute
)

type Message struct {
	ID                 string        `json:"id" gorm:"type:uuid;primaryKey"`
	Sequence           int64         `json:"sequence,string" gorm:"autoIncrement"`
	ConferenceID       string        `json:"conferenceId"`
	SenderID           string        `json:"senderId" gorm:"column:sender_user_id"`
	SenderName         string        `json:"senderName" gorm:"->;-:migration"`
	ClientRequestID    string        `json:"clientRequestId"`
	RequestFingerprint string        `json:"-"`
	Text               string        `json:"text"`
	ReplyTo            *string       `json:"replyTo" gorm:"column:reply_to_id"`
	ReplyPreview       *ReplyPreview `json:"replyPreview,omitempty" gorm:"-"`
	Version            int64         `json:"version"`
	CreatedAt          time.Time     `json:"createdAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
	DeletedAt          *time.Time    `json:"deletedAt"`
	Attachments        []Attachment  `json:"attachments" gorm:"-"`
}

func (Message) TableName() string { return "chat_messages" }

type ReplyPreview struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	SenderName string `json:"senderName"`
	Deleted    bool   `json:"deleted"`
}
type Attachment struct {
	ID               string     `json:"id" gorm:"type:uuid;primaryKey"`
	ConferenceID     string     `json:"conferenceId"`
	OwnerID          string     `json:"ownerId" gorm:"column:owner_user_id"`
	ClientRequestID  string     `json:"clientRequestId"`
	Filename         string     `json:"filename"`
	MimeType         string     `json:"mimeType"`
	Size             int64      `json:"size"`
	Checksum         string     `json:"checksum,omitempty"`
	Status           string     `json:"status"`
	MessageID        *string    `json:"messageId,omitempty"`
	ObjectKey        string     `json:"-"`
	UploadToken      *string    `json:"-"`
	UploadLeaseUntil *time.Time `json:"-"`
	UploadedAt       *time.Time `json:"-"`
	CleanedAt        *time.Time `json:"-"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	ExpiresAt        time.Time  `json:"expiresAt"`
}

func (Attachment) TableName() string { return "chat_attachments" }
func (a Attachment) Prefix() string  { return "attachments/" + a.ConferenceID + "/" + a.ID + "/" }

type SendRequest struct {
	ClientRequestID string   `json:"clientRequestId"`
	Text            string   `json:"text"`
	ReplyTo         string   `json:"replyTo,omitempty"`
	AttachmentIDs   []string `json:"attachmentIds,omitempty"`
}
type EditRequest struct {
	Text string `json:"text"`
}
type InitRequest struct {
	ClientRequestID string `json:"clientRequestId"`
	Filename        string `json:"filename"`
	MimeType        string `json:"mimeType"`
	Size            int64  `json:"size"`
}
type ReadRequest struct {
	MessageID string `json:"messageId"`
}
type ReadState struct {
	LastReadMessageID *string `json:"lastReadMessageId"`
	LastReadSequence  int64   `json:"-"`
	UnreadCount       int64   `json:"unreadCount"`
	ParticipantID     string  `json:"-"`
}
type Page struct {
	Items             []Message `json:"items"`
	NextCursor        string    `json:"nextCursor,omitempty"`
	UnreadCount       int64     `json:"unreadCount"`
	LastReadMessageID *string   `json:"lastReadMessageId"`
}

func UUID(value string) (string, error) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil {
		return "", apperrors.New(apperrors.ErrInvalidInput, "a non-zero UUID is required")
	}
	return id.String(), nil
}
func NormalizeText(text string, allowEmpty bool) (string, error) {
	text = strings.TrimSpace(text)
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > MaxTextRunes || (!allowEmpty && text == "") || strings.ContainsRune(text, 0) {
		return "", apperrors.New(apperrors.ErrInvalidInput, "message must contain 1 to 4000 characters or an attachment")
	}
	return text, nil
}
func NormalizeSend(request SendRequest) (SendRequest, string, error) {
	var err error
	request.ClientRequestID, err = UUID(request.ClientRequestID)
	if err != nil {
		return request, "", err
	}
	if len(request.AttachmentIDs) > MaxAttachments {
		return request, "", apperrors.New(apperrors.ErrInvalidInput, "at most 5 attachments are allowed")
	}
	request.AttachmentIDs = append([]string(nil), request.AttachmentIDs...)
	seen := map[string]bool{}
	for i, id := range request.AttachmentIDs {
		id, err = UUID(id)
		if err != nil {
			return request, "", err
		}
		if seen[id] {
			return request, "", apperrors.New(apperrors.ErrInvalidInput, "duplicate attachment")
		}
		seen[id] = true
		request.AttachmentIDs[i] = id
	}
	sort.Strings(request.AttachmentIDs)
	if request.ReplyTo != "" {
		request.ReplyTo, err = UUID(request.ReplyTo)
		if err != nil {
			return request, "", err
		}
	}
	request.Text, err = NormalizeText(request.Text, len(request.AttachmentIDs) > 0)
	if err != nil {
		return request, "", err
	}
	raw, _ := json.Marshal(request)
	sum := sha256.Sum256(raw)
	return request, hex.EncodeToString(sum[:]), nil
}
func NormalizeInit(request InitRequest) (InitRequest, error) {
	var err error
	request.ClientRequestID, err = UUID(request.ClientRequestID)
	if err != nil {
		return request, err
	}
	request.Filename = strings.TrimSpace(request.Filename)
	if !utf8.ValidString(request.Filename) || request.Filename == "" || utf8.RuneCountInString(request.Filename) > 180 || filepath.Base(request.Filename) != request.Filename || strings.ContainsAny(request.Filename, "/\\") || strings.Contains(request.Filename, "..") {
		return request, apperrors.New(apperrors.ErrInvalidInput, "invalid filename")
	}
	for _, r := range request.Filename {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return request, apperrors.New(apperrors.ErrInvalidInput, "invalid filename")
		}
	}
	types := map[string]string{".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".webp": "image/webp", ".pdf": "application/pdf", ".txt": "text/plain", ".csv": "text/csv"}
	expected, ok := types[strings.ToLower(filepath.Ext(request.Filename))]
	if !ok {
		return request, apperrors.New(apperrors.ErrInvalidInput, "only JPG, PNG, WebP, PDF, TXT and CSV attachments are supported")
	}
	if request.MimeType != "" && request.MimeType != expected && !(expected == "text/csv" && request.MimeType == "text/plain") {
		return request, apperrors.New(apperrors.ErrInvalidInput, "filename and MIME type disagree")
	}
	request.MimeType = expected
	if request.Size < 1 || request.Size > MaxAttachmentBytes {
		return request, apperrors.New(apperrors.ErrInvalidInput, fmt.Sprintf("attachment size must be between 1 and %d bytes", MaxAttachmentBytes))
	}
	return request, nil
}

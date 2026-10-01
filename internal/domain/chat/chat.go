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

// Message сохраняет сообщение чата, порядок, версию, автора и ключ повторной отправки; внутренний отпечаток не входит в JSON.
//   - ID: уникальный идентификатор данной сущности.
//   - Sequence: серверный монотонный номер сообщения или команды.
//   - ConferenceID: идентификатор конференции, ограничивающий область операции.
//   - SenderID: идентификатор отправителя сообщения.
//   - SenderName: сохранённое отображаемое имя отправителя.
//   - ClientRequestID: стабильный ключ клиентской операции для защиты повторов от дублей.
//   - RequestFingerprint: SHA-256 нормализованных параметров запроса для сравнения повторной операции.
//   - Text: обычный текст сообщения, подлежащий проверке или обработке.
//   - ReplyTo: идентификатор исходного запроса или сообщения, на которое даётся ответ.
//   - ReplyPreview: краткое представление исходного сообщения, учитывающее его удаление.
//   - Version: версия изменения для защиты от устаревших операций.
//   - CreatedAt: время создания значения.
//   - UpdatedAt: время последнего сохранённого изменения.
//   - DeletedAt: время мягкого удаления; nil означает действующее значение.
//   - Attachments: метаданные файлов, прикреплённых к сообщению.
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

// TableName возвращает точное имя таблицы для GORM, чтобы модель не зависела от автоматического образования имени.
//
// @return:
//   - результат 1 (string): имя таблицы, используемое ORM.
func (Message) TableName() string { return "chat_messages" }

// ReplyPreview передаёт краткое представление исходного сообщения для ответа, учитывая мягкое удаление.
//   - ID: уникальный идентификатор данной сущности.
//   - Text: обычный текст сообщения, подлежащий проверке или обработке.
//   - SenderName: сохранённое отображаемое имя отправителя.
//   - Deleted: признак мягкого удаления исходного сообщения.
type ReplyPreview struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	SenderName string `json:"senderName"`
	Deleted    bool   `json:"deleted"`
}

// Attachment хранит владельца, конференцию, параметры объекта и состояние загрузки; внутренний путь и аренда скрыты от JSON.
//   - ID: уникальный идентификатор данной сущности.
//   - ConferenceID: идентификатор конференции, ограничивающий область операции.
//   - OwnerID: идентификатор организатора или владельца ресурса.
//   - ClientRequestID: стабильный ключ клиентской операции для защиты повторов от дублей.
//   - Filename: проверяемое или формируемое имя файла без управляемого пользователем пути.
//   - MimeType: заявленный либо проверенный MIME-тип содержимого.
//   - Size: размер содержимого в байтах.
//   - Checksum: контрольная сумма содержимого для проверки неизменности передачи.
//   - Status: состояние ресурса, ответа или фильтра выборки.
//   - MessageID: идентификатор сообщения внутри конференции.
//   - ObjectKey: серверный ключ объекта внутри приватного бакета.
//   - UploadToken: идентификатор текущей попытки загрузки, защищающий от устаревшего результата.
//   - UploadLeaseUntil: конец права текущей попытки загрузки изменять вложение.
//   - UploadedAt: время подтверждённой передачи объекта.
//   - CleanedAt: время завершения очистки просроченного вложения.
//   - CreatedAt: время создания значения.
//   - UpdatedAt: время последнего сохранённого изменения.
//   - ExpiresAt: момент окончания действия сессии, токена или аренды.
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

// TableName возвращает точное имя таблицы для GORM, чтобы модель не зависела от автоматического образования имени.
//
// @return:
//   - результат 1 (string): имя таблицы, используемое ORM.
func (Attachment) TableName() string { return "chat_attachments" }

// Prefix строит серверный префикс объектов вложения внутри своей конференции.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func (a Attachment) Prefix() string { return "attachments/" + a.ConferenceID + "/" + a.ID + "/" }

// SendRequest передаёт текст, ссылку ответа и идентификаторы вложений вместе с ключом повторной отправки.
// Состав:
//   - ClientRequestID: стабильный ключ клиентской операции для защиты повторов от дублей.
//   - Text: обычный текст сообщения, подлежащий проверке или обработке.
//   - ReplyTo: идентификатор исходного запроса или сообщения, на которое даётся ответ.
//   - AttachmentIDs: идентификаторы подготовленных вложений для атомарной привязки.
type SendRequest struct {
	ClientRequestID string   `json:"clientRequestId"`
	Text            string   `json:"text"`
	ReplyTo         string   `json:"replyTo,omitempty"`
	AttachmentIDs   []string `json:"attachmentIds,omitempty"`
}

// EditRequest передаёт новый текст редактируемого сообщения.
//   - Text: обычный текст сообщения, подлежащий проверке или обработке.
type EditRequest struct {
	Text string `json:"text"`
}

// InitRequest передаёт параметры будущего вложения до выдачи серверного ключа загрузки.
//   - ClientRequestID: стабильный ключ клиентской операции для защиты повторов от дублей.
//   - Filename: проверяемое или формируемое имя файла без управляемого пользователем пути.
//   - MimeType: заявленный либо проверенный MIME-тип содержимого.
//   - Size: размер содержимого в байтах.
type InitRequest struct {
	ClientRequestID string `json:"clientRequestId"`
	Filename        string `json:"filename"`
	MimeType        string `json:"mimeType"`
	Size            int64  `json:"size"`
}

// ReadRequest передаёт сообщение, до которого пользователь прочитал чат.
//   - MessageID: идентификатор сообщения внутри конференции.
type ReadRequest struct {
	MessageID string `json:"messageId"`
}

// ReadState передаёт монотонную границу прочтения и число непрочитанных сообщений.
//   - LastReadMessageID: идентификатор сообщения сохранённой границы прочтения.
//   - LastReadSequence: монотонный номер последнего прочитанного сообщения.
//   - UnreadCount: число доступных непрочитанных элементов.
//   - ParticipantID: идентификатор членства участника внутри конференции.
type ReadState struct {
	LastReadMessageID *string `json:"lastReadMessageId"`
	LastReadSequence  int64   `json:"-"`
	UnreadCount       int64   `json:"unreadCount"`
	ParticipantID     string  `json:"-"`
}

// Page собирает элементы одной страницы и метаданные продолжения списка.
//   - Items: элементы страницы или порции пакетной обработки.
//   - NextCursor: непрозрачная граница следующей страницы; пустое значение завершает список.
//   - UnreadCount: число доступных непрочитанных элементов.
//   - LastReadMessageID: идентификатор сообщения сохранённой границы прочтения.
type Page struct {
	Items             []Message `json:"items"`
	NextCursor        string    `json:"nextCursor,omitempty"`
	UnreadCount       int64     `json:"unreadCount"`
	LastReadMessageID *string   `json:"lastReadMessageId"`
}

// UUID проверяет ненулевой UUID и возвращает его каноническую строковую форму.
//
// @parameters:
//   - value (string): значение для проверки, нормализации или преобразования.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func UUID(value string) (string, error) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil {
		return "", apperrors.New(apperrors.ErrInvalidInput, "a non-zero UUID is required")
	}
	return id.String(), nil
}

// NormalizeText обрезает пробелы, проверяет UTF-8, отсутствие нулевого символа и предел 4000 символов текста сообщения.
//
// @parameters:
//   - text (string): обычный текст сообщения, подлежащий проверке или обработке.
//   - allowEmpty (bool): разрешает пустой текст, когда содержимое представлено вложением.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func NormalizeText(text string, allowEmpty bool) (string, error) {
	text = strings.TrimSpace(text)
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > MaxTextRunes || (!allowEmpty && text == "") || strings.ContainsRune(text, 0) {
		return "", apperrors.New(apperrors.ErrInvalidInput, "message must contain 1 to 4000 characters or an attachment")
	}
	return text, nil
}

// NormalizeSend проверяет запрос сообщения, нормализует UUID и текст, сортирует вложения и вычисляет SHA-256 для проверки повторной отправки.
//
// @parameters:
//   - request (SendRequest): входные параметры соответствующего прикладного запроса.
//
// @return:
//   - результат 1 (SendRequest): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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

// NormalizeInit проверяет имя, расширение, MIME и размер вложения, нормализует ключ повторной операции и исключает пути и управляющие символы.
//
// @parameters:
//   - request (InitRequest): входные параметры соответствующего прикладного запроса.
//
// @return:
//   - результат 1 (InitRequest): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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

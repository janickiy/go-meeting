package s3

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

// CheckAttachmentPrivacy проверяет отсутствие публичной политики бакета для приватных вложений.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Client) CheckAttachmentPrivacy(ctx context.Context) error {
	policy, err := c.minio.GetBucketPolicy(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("check private attachment bucket: %w", err)
	}
	if strings.TrimSpace(policy) != "" {
		return fmt.Errorf("attachments require a private bucket without a bucket policy")
	}
	return nil
}

// PutAttachment сохраняет байты вложения по ключу, сформированному сервером.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - key (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
//   - reader (io.Reader): источник содержимого либо читатель карточек записи согласно типу.
//   - size (int64): размер содержимого в байтах.
//   - contentType (string): значение contentType типа string, используемое согласно назначению этой операции.
//   - checksum (string): контрольная сумма содержимого для проверки неизменности передачи.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Client) PutAttachment(ctx context.Context, key string, reader io.Reader, size int64, contentType, checksum string) error {
	if !attachmentKey(key) {
		return fmt.Errorf("invalid attachment key")
	}
	_, err := c.minio.PutObject(ctx, c.bucket, key, reader, size, minio.PutObjectOptions{ContentType: contentType, UserMetadata: map[string]string{"sha256": checksum}})
	return err
}

// StatAttachment читает фактический размер и метаданные объекта перед финализацией.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - key (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
//
// @return:
//   - результат 1 (int64): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 3 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 4 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Client) StatAttachment(ctx context.Context, key string) (int64, string, string, error) {
	if !attachmentKey(key) {
		return 0, "", "", fmt.Errorf("invalid attachment key")
	}
	object, err := c.minio.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return 0, "", "", err
	}
	return object.Size, object.ContentType, object.UserMetadata["Sha256"], nil
}

// AttachmentDownloadURL создаёт краткоживущую подписанную ссылку с принудительным скачиванием файла.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - key (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
//   - filename (string): проверяемое или формируемое имя файла без управляемого пользователем пути.
//   - expiry (time.Duration): значение expiry типа time.Duration, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (string): адрес разрешённого чтения или целевого ресурса.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Client) AttachmentDownloadURL(ctx context.Context, key, filename string, expiry time.Duration) (string, error) {
	if !attachmentKey(key) || expiry <= 0 || expiry > 5*time.Minute {
		return "", fmt.Errorf("invalid attachment download parameters")
	}
	client := c.minio
	if c.presignMinio != nil {
		client = c.presignMinio
	}
	params := url.Values{}
	params.Set("response-content-disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	params.Set("response-content-type", "application/octet-stream")
	params.Set("response-cache-control", "private, no-store")
	objectURL, err := client.PresignedGetObject(ctx, c.bucket, key, expiry, params)
	if err != nil {
		return "", err
	}
	return objectURL.String(), nil
}

// CleanAttachmentObjects удаляет объекты точного префикса вложения, сохраняя выигравший прикреплённый объект.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - prefix (string): ограниченный префикс объектов, относящихся к одной операции.
//   - keep (string): объект или значение, которое необходимо сохранить при очистке.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Client) CleanAttachmentObjects(ctx context.Context, prefix, keep string) error {
	parts := attachmentParts(strings.TrimSuffix(prefix, "/"))
	if len(parts) != 3 || parts[0] != "attachments" || !attachmentID(parts[1]) || !attachmentID(parts[2]) || !strings.HasSuffix(prefix, "/") || (keep != "" && (!attachmentKey(keep) || !strings.HasPrefix(keep, prefix))) {
		return fmt.Errorf("invalid attachment cleanup prefix")
	}
	for object := range c.minio.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if object.Err != nil {
			return object.Err
		}
		if object.Key == keep {
			continue
		}
		if err := c.minio.RemoveObject(ctx, c.bucket, object.Key, minio.RemoveObjectOptions{}); err != nil {
			return err
		}
	}
	return nil
}

// attachmentID проверяет канонический UUID для безопасного построения ключа объекта.
//
// @args
//   - value (string): значение для проверки, нормализации или преобразования.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func attachmentID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

// attachmentKey проверяет структуру серверного пути вложения и идентификаторы его сегментов.
//
// @args
//   - key (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func attachmentKey(key string) bool {
	parts := attachmentParts(key)
	return len(parts) == 4 && parts[0] == "attachments" && attachmentID(parts[1]) && attachmentID(parts[2]) && attachmentID(parts[3])
}

// Direct files have a distinct prefix; both scopes still require canonical UUIDs.
func attachmentParts(key string) []string {
	parts := strings.Split(key, "/")
	if len(parts) > 1 && parts[1] == "direct" {
		parts = append(parts[:1], parts[2:]...)
	}
	return parts
}

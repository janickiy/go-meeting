package s3

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client объединяет настройки и соединения клиента соответствующего внешнего сервиса.
//   - minio: значение minio типа *minio.Client, используемое согласно назначению этой операции.
//   - presignMinio: значение presignMinio типа *minio.Client, используемое согласно назначению этой операции.
//   - bucket: имя бакета объектного хранилища.
//   - endpoint: адрес конечной точки вызываемого сервиса.
//   - accessKey: значение accessKey типа string, используемое согласно назначению этой операции.
//   - secretKey: значение secretKey типа string, используемое согласно назначению этой операции.
//   - useSSL: логический признак useSSL, управляющий соответствующей веткой обработки.
type Client struct {
	minio        *minio.Client
	presignMinio *minio.Client
	bucket       string
	endpoint     string
	accessKey    string
	secretKey    string
	useSSL       bool
}

// UploadedObject возвращает путь, размер и метаданные объекта после загрузки артефакта.
//   - Bucket: имя бакета объектного хранилища.
//   - ObjectKey: серверный ключ объекта внутри приватного бакета.
//   - SizeBytes: фактический размер объекта в байтах.
type UploadedObject struct {
	Bucket    string
	ObjectKey string
	SizeBytes int64
}

// CompletedRecord собирает найденные в бакете итоговые объекты одной завершённой записи.
//   - RecordID: внешний UUID задачи записи.
//   - Status: состояние ресурса, ответа или фильтра выборки.
//   - Bucket: имя бакета объектного хранилища.
//   - PreviewObjectKey: значение PreviewObjectKey типа string, используемое согласно назначению этой операции.
//   - PreviewURL: значение PreviewURL типа string, используемое согласно назначению этой операции.
//   - FinalObjectKey: значение FinalObjectKey типа string, используемое согласно назначению этой операции.
//   - FinalURL: значение FinalURL типа string, используемое согласно назначению этой операции.
//   - SizeBytes: фактический размер объекта в байтах.
//   - LastModified: временная отметка LastModified; указатель допускает отсутствие значения.
type CompletedRecord struct {
	RecordID         string    `json:"recordId"`
	Status           string    `json:"status"`
	Bucket           string    `json:"bucket"`
	PreviewObjectKey string    `json:"previewObjectKey,omitempty"`
	PreviewURL       string    `json:"previewUrl,omitempty"`
	FinalObjectKey   string    `json:"finalObjectKey,omitempty"`
	FinalURL         string    `json:"finalUrl,omitempty"`
	SizeBytes        int64     `json:"sizeBytes,omitempty"`
	LastModified     time.Time `json:"lastModified,omitempty"`
}

// NewClient создает S3-клиент и bucket при необходимости.
// @parameters:
// - endpoint: host:port MinIO.
// - accessKey: access key.
// - secretKey: secret key.
// - bucket: bucket для записей.
// - useSSL: использовать HTTPS.
// @return Client или ошибку подключения.
func NewClient(ctx context.Context, endpoint string, accessKey string, secretKey string, bucket string, useSSL bool) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	client, err := minio.New(endpoint, &minio.Options{
		Creds:      credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:     useSSL,
		Transport:  boundedTransport(),
		MaxRetries: 3,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("check minio bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create minio bucket: %w", err)
		}
	}

	return &Client{
		minio:     client,
		bucket:    bucket,
		endpoint:  endpoint,
		accessKey: accessKey,
		secretKey: secretKey,
		useSSL:    useSSL,
	}, nil
}

// boundedTransport ограничивает ожидание соединения и заголовков MinIO, не
// ограничивая общую длительность передачи большой записи сверх контекста.
// Аргументов нет; возвращает отдельный пул HTTP-соединений.
func boundedTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	t.ResponseHeaderTimeout = 10 * time.Second
	t.TLSHandshakeTimeout = 5 * time.Second
	t.MaxIdleConnsPerHost = 8
	return t
}

// Check проверяет доступность приватного бакета в ограниченном контексте ctx.
// Возвращает ошибку, если бакет недоступен или исчез; объекты не перечисляются.
func (c *Client) Check(ctx context.Context) error {
	exists, err := c.minio.BucketExists(ctx, c.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("storage bucket unavailable")
	}
	return nil
}

// SetPublicEndpoint задает внешний endpoint MinIO для ссылок, которые открываются с хоста.
// @parameters:
// - endpoint: host:port или URL, например localhost:9000.
// @return ничего.
func (c *Client) SetPublicEndpoint(endpoint string) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		c.presignMinio = nil
		return
	}
	secure := c.useSSL
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Host == "" {
			c.presignMinio = nil
			return
		}
		endpoint = parsed.Host
		secure = parsed.Scheme == "https"
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(c.accessKey, c.secretKey, ""),
		Secure:    secure,
		Region:    "us-east-1",
		Transport: c.publicEndpointTransport(endpoint),
	})
	if err != nil {
		c.presignMinio = nil
		return
	}
	c.presignMinio = client
}

// publicEndpointTransport создаёт транспорт запросов подписания с корректным внешним адресом хранилища.
//
// @parameters:
//   - publicEndpoint (string): значение publicEndpoint типа string, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (http.RoundTripper): значение, подготовленное операцией для вызывающей стороны.
func (c *Client) publicEndpointTransport(publicEndpoint string) http.RoundTripper {
	transport := boundedTransport()
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	internalEndpoint := c.endpoint
	// Вложенный обработчик выполняет выделенный шаг обработки в хранении и очистке артефактов, используя состояние окружающей функции.
	//
	// @parameters:
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - network (string): значение network типа string, используемое согласно назначению этой операции.
	//   - address (string): адрес целевого внутреннего сервиса или сети.
	//
	// @return:
	//   - результат 1 (net.Conn): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		if address == publicEndpoint && internalEndpoint != "" {
			address = internalEndpoint
		}

		return dialer.DialContext(ctx, network, address)
	}

	return transport
}

// Bucket возвращает имя bucket, куда сохраняются записи.
// @parameters: нет.
// @return имя bucket.
func (c *Client) Bucket() string {
	return c.bucket
}

// UploadFile загружает локальный файл в bucket.
// @parameters:
// - ctx: контекст операции.
// - objectKey: ключ объекта в bucket.
// - path: локальный путь.
// - contentType: MIME-тип.
// @return UploadedObject или ошибку upload.
func (c *Client) UploadFile(ctx context.Context, objectKey string, path string, contentType string) (UploadedObject, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	local, err := os.Stat(path)
	if err != nil {
		return UploadedObject{}, err
	}
	info, err := c.minio.FPutObject(ctx, c.bucket, objectKey, path, minio.PutObjectOptions{
		ContentType: contentType,
		UserMetadata: map[string]string{
			"file-name": filepath.Base(path),
		},
	})
	if err != nil {
		return UploadedObject{}, fmt.Errorf("upload %s to minio: %w", objectKey, err)
	}
	verified, err := c.minio.StatObject(ctx, c.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil || verified.Size != local.Size() {
		return UploadedObject{}, fmt.Errorf("uploaded object verification failed")
	}

	return UploadedObject{Bucket: c.bucket, ObjectKey: objectKey, SizeBytes: info.Size}, nil
}

// RemovePrefix удаляет все объекты по prefix из bucket.
// @parameters:
// - ctx: контекст операции.
// - prefix: префикс объектов, например records/{recordId}/segments/.
// @return ошибку удаления первого проблемного объекта.
func (c *Client) RemovePrefix(ctx context.Context, prefix string) error {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}
	objects := c.minio.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})
	for removeErr := range c.minio.RemoveObjects(ctx, c.bucket, objects, minio.RemoveObjectsOptions{}) {
		if removeErr.Err != nil {
			return fmt.Errorf("remove %s from minio: %w", removeErr.ObjectName, removeErr.Err)
		}
	}

	return nil
}

// ListCompletedRecords возвращает записи, у которых в MinIO есть final.mp4 и preview.jpg.
// @parameters:
// - ctx: контекст операции.
// - limit: максимум записей в ответе.
// @return список завершенных записей с presigned URLs или ошибку MinIO.
func (c *Client) ListCompletedRecords(ctx context.Context, limit int) ([]CompletedRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	byRecordID := make(map[string]*CompletedRecord)
	objects := c.minio.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{
		Prefix:    "records/",
		Recursive: true,
	})
	for object := range objects {
		if object.Err != nil {
			return nil, fmt.Errorf("list completed records from minio: %w", object.Err)
		}
		recordID, fileName, ok := completedRecordObject(object.Key)
		if !ok {
			continue
		}
		item := byRecordID[recordID]
		if item == nil {
			item = &CompletedRecord{
				RecordID: recordID,
				Status:   "ready",
				Bucket:   c.bucket,
			}
			byRecordID[recordID] = item
		}
		if object.LastModified.After(item.LastModified) {
			item.LastModified = object.LastModified
		}
		if fileName == "final.mp4" {
			item.FinalObjectKey = object.Key
			item.SizeBytes = object.Size
		}
		if fileName == "preview.jpg" {
			item.PreviewObjectKey = object.Key
		}
	}

	result := make([]CompletedRecord, 0, len(byRecordID))
	for _, item := range byRecordID {
		if item.FinalObjectKey == "" || item.PreviewObjectKey == "" {
			continue
		}
		if item.FinalObjectKey != "" {
			url, err := c.PresignedGetURL(ctx, item.FinalObjectKey, 24*time.Hour)
			if err != nil {
				return nil, err
			}
			item.FinalURL = url
		}
		if item.PreviewObjectKey != "" {
			url, err := c.PresignedGetURL(ctx, item.PreviewObjectKey, 24*time.Hour)
			if err != nil {
				return nil, err
			}
			item.PreviewURL = url
		}
		result = append(result, *item)
	}
	sort.Slice(result, /* Вложенный обработчик выполняет выделенный шаг обработки в хранении и очистке артефактов, используя состояние окружающей функции.

		@parameters:
		  - i (int): значение i типа int, используемое согласно назначению этой операции.
		  - j (int): значение j типа int, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(i int, j int) bool {
			return result[i].LastModified.After(result[j].LastModified)
		})
	if len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// completedRecordObject разбирает ключ объекта готовой записи и определяет его роль.
//
// @parameters:
//   - key (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 3 (bool): признак выполнения проверяемого условия или изменения состояния.
func completedRecordObject(key string) (string, string, bool) {
	parts := strings.Split(strings.Trim(key, "/"), "/")
	if len(parts) != 3 || parts[0] != "records" {
		return "", "", false
	}
	if parts[2] != "final.mp4" && parts[2] != "preview.jpg" {
		return "", "", false
	}

	return parts[1], parts[2], true
}

// PresignedGetURL создает временную ссылку на объект MinIO.
// @parameters:
// - ctx: контекст операции.
// - objectKey: ключ объекта в bucket.
// - expiry: срок жизни ссылки.
// @return URL для скачивания/просмотра объекта или ошибку MinIO.
func (c *Client) PresignedGetURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	if strings.TrimSpace(objectKey) == "" {
		return "", nil
	}
	client := c.minio
	if c.presignMinio != nil {
		client = c.presignMinio
	}
	objectURL, err := client.PresignedGetObject(ctx, c.bucket, objectKey, expiry, url.Values{})
	if err != nil {
		return "", fmt.Errorf("presign %s: %w", objectKey, err)
	}

	return objectURL.String(), nil
}

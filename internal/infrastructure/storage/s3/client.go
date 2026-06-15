package s3

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client загружает артефакты записи в MinIO/S3.
type Client struct {
	minio        *minio.Client
	presignMinio *minio.Client
	bucket       string
	endpoint     string
	accessKey    string
	secretKey    string
	useSSL       bool
}

// UploadedObject описывает загруженный объект.
type UploadedObject struct {
	Bucket    string
	ObjectKey string
	SizeBytes int64
}

// CompletedRecord описывает завершенную запись, найденную напрямую в MinIO.
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
// Параметры:
// - endpoint: host:port MinIO.
// - accessKey: access key.
// - secretKey: secret key.
// - bucket: bucket для записей.
// - useSSL: использовать HTTPS.
// Возвращает: Client или ошибку подключения.
func NewClient(ctx context.Context, endpoint string, accessKey string, secretKey string, bucket string, useSSL bool) (*Client, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
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

// SetPublicEndpoint задает внешний endpoint MinIO для ссылок, которые открываются с хоста.
// Параметры:
// - endpoint: host:port или URL, например localhost:9000.
// Возвращает: ничего.
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

func (c *Client) publicEndpointTransport(publicEndpoint string) http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	internalEndpoint := c.endpoint
	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		if address == publicEndpoint && internalEndpoint != "" {
			address = internalEndpoint
		}

		return dialer.DialContext(ctx, network, address)
	}

	return transport
}

// Bucket возвращает имя bucket, куда сохраняются записи.
// Параметры: нет.
// Возвращает: имя bucket.
func (c *Client) Bucket() string {
	return c.bucket
}

// UploadFile загружает локальный файл в bucket.
// Параметры:
// - ctx: контекст операции.
// - objectKey: ключ объекта в bucket.
// - path: локальный путь.
// - contentType: MIME-тип.
// Возвращает: UploadedObject или ошибку upload.
func (c *Client) UploadFile(ctx context.Context, objectKey string, path string, contentType string) (UploadedObject, error) {
	info, err := c.minio.FPutObject(ctx, c.bucket, objectKey, path, minio.PutObjectOptions{
		ContentType: contentType,
		UserMetadata: map[string]string{
			"file-name": filepath.Base(path),
		},
	})
	if err != nil {
		return UploadedObject{}, fmt.Errorf("upload %s to minio: %w", objectKey, err)
	}

	return UploadedObject{Bucket: c.bucket, ObjectKey: objectKey, SizeBytes: info.Size}, nil
}

// RemovePrefix удаляет все объекты по prefix из bucket.
// Параметры:
// - ctx: контекст операции.
// - prefix: префикс объектов, например records/{recordId}/segments/.
// Возвращает: ошибку удаления первого проблемного объекта.
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
// Параметры:
// - ctx: контекст операции.
// - limit: максимум записей в ответе.
// Возвращает: список завершенных записей с presigned URLs или ошибку MinIO.
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
	sort.Slice(result, func(i int, j int) bool {
		return result[i].LastModified.After(result[j].LastModified)
	})
	if len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

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
// Параметры:
// - ctx: контекст операции.
// - objectKey: ключ объекта в bucket.
// - expiry: срок жизни ссылки.
// Возвращает: URL для скачивания/просмотра объекта или ошибку MinIO.
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

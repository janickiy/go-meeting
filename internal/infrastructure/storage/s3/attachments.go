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

// CheckAttachmentPrivacy is deliberately read-only: collaboration must not
// silently inherit a public bucket or rewrite an operator's storage policy.
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

func (c *Client) PutAttachment(ctx context.Context, key string, reader io.Reader, size int64, contentType, checksum string) error {
	if !attachmentKey(key) {
		return fmt.Errorf("invalid attachment key")
	}
	_, err := c.minio.PutObject(ctx, c.bucket, key, reader, size, minio.PutObjectOptions{ContentType: contentType, UserMetadata: map[string]string{"sha256": checksum}})
	return err
}
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

// CleanAttachmentObjects only accepts a complete generated attachment prefix.
// Keeping the winning immutable object also removes abandoned upload attempts
// when an attachment eventually becomes attached to a message.
func (c *Client) CleanAttachmentObjects(ctx context.Context, prefix, keep string) error {
	parts := strings.Split(strings.TrimSuffix(prefix, "/"), "/")
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

func attachmentID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}
func attachmentKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) == 4 && parts[0] == "attachments" && attachmentID(parts[1]) && attachmentID(parts[2]) && attachmentID(parts[3])
}

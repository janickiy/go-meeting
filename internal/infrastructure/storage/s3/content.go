package s3

import (
	"context"
	"fmt"
	"github.com/minio/minio-go/v7"
	"io"
	"strings"
)

// limitedRecording сохраняет ограничение чтения и закрывает приватный объект.
type limitedRecording struct {
	reader io.Reader
	object io.Closer
}

// Read читает только заранее ограниченный объект.
// @args p — буфер вызывающей стороны.
// @return число байтов и ошибку потока либо EOF.
func (r *limitedRecording) Read(p []byte) (int, error) { return r.reader.Read(p) }

// Close освобождает HTTP/MinIO ресурсы потока.
// @return ошибка закрытия private object.
func (r *limitedRecording) Close() error { return r.object.Close() }

// OpenRecording открывает приватный готовый MP4 без публичного URL и произвольных сетевых запросов.
// @args ctx — срок выполнения воркера; key — серверный ключ из БД; maximum — бюджет байтов.
// @return ограниченный поток, подтверждённый размер и безопасную ошибку хранилища.
func (c *Client) OpenRecording(ctx context.Context, key string, maximum int64) (io.ReadCloser, int64, error) {
	parts := strings.Split(key, "/")
	if maximum <= 0 || len(parts) != 6 || parts[0] != "recordings" || !attachmentID(parts[1]) || !attachmentID(parts[2]) || parts[3] != "artifacts" || !attachmentID(parts[4]) || parts[5] != "final.mp4" {
		return nil, 0, fmt.Errorf("invalid private recording key")
	}
	object, err := c.minio.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("private recording unavailable")
	}
	info, err := object.Stat()
	if err != nil {
		object.Close()
		return nil, 0, fmt.Errorf("private recording unavailable")
	}
	if info.Size <= 0 || info.Size > maximum {
		object.Close()
		return nil, 0, fmt.Errorf("recording input exceeds budget")
	}
	return &limitedRecording{reader: io.LimitReader(object, maximum+1), object: object}, info.Size, nil
}

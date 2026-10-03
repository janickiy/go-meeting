// Команда object-backup копирует текущие версии приватных объектов и проверяет восстановление.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// object описывает один файл резервной копии без возможности задать произвольный локальный путь.
// @params: Key — ключ хранилища; File — локальное имя; SHA256 — контрольная сумма;
// Size — число байтов; ContentType — исходный тип содержимого.
type object struct {
	Key         string `json:"key"`
	File        string `json:"file"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	ContentType string `json:"contentType"`
}

// manifest связывает копию текущих объектов с исходным бакетом и временем создания.
// @params: SchemaVersion — версия формата; Bucket — источник; CreatedAt — время UTC; Objects — состав копии.
type manifest struct {
	SchemaVersion int      `json:"schemaVersion"`
	Bucket        string   `json:"bucket"`
	CreatedAt     string   `json:"createdAt"`
	Objects       []object `json:"objects"`
}

// main запускает выбранную операцию с конечным сроком и не выводит параметры доступа или ключи объектов.
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "object backup failed:", err)
		os.Exit(1)
	}
}

// run разбирает параметры и создаёт изолированный клиент по внешним переменным окружения.
// @args args — операция backup/restore/verify, каталог и необязательный целевой бакет.
// @return безопасную причину отказа без исходных ответов хранилища.
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("operation backup, restore or verify is required")
	}
	flags := flag.NewFlagSet("object-backup", flag.ContinueOnError)
	dir := flags.String("dir", "", "private backup directory")
	bucket := flags.String("bucket", os.Getenv("MINIO_BUCKET"), "source or empty restore bucket")
	timeout := flags.Duration("timeout", time.Hour, "total operation deadline")
	if flags.Parse(args[1:]) != nil || *dir == "" || *bucket == "" || *timeout <= 0 || *timeout > 24*time.Hour {
		return errors.New("valid directory, bucket and timeout are required")
	}
	endpoint, user, password := os.Getenv("MINIO_ENDPOINT"), os.Getenv("MINIO_ROOT_USER"), os.Getenv("MINIO_ROOT_PASSWORD")
	if endpoint == "" || user == "" || password == "" {
		return errors.New("external storage credentials are required")
	}
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(user, password, ""), Secure: os.Getenv("MINIO_USE_SSL") == "true"})
	if err != nil {
		return errors.New("invalid storage configuration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	switch args[0] {
	case "backup":
		return backup(ctx, client, *bucket, *dir)
	case "restore", "verify":
		m, err := readManifest(*dir)
		if err != nil {
			return err
		}
		if args[0] == "restore" {
			if err = restore(ctx, client, *bucket, *dir, m); err != nil {
				return err
			}
		}
		return verify(ctx, client, *bucket, m)
	default:
		return errors.New("unknown operation")
	}
}

// backup создаёт новую копию и публикует манифест только после полного успешного чтения.
// @args ctx — срок операции; client — S3-клиент; bucket — источник; dir — новый приватный каталог.
// @return ошибку чтения или записи без ключей объектов и секретов.
func backup(ctx context.Context, client *minio.Client, bucket, dir string) error {
	if err := os.Mkdir(dir, 0700); err != nil {
		return errors.New("backup directory must not already exist")
	}
	m := manifest{SchemaVersion: 1, Bucket: bucket, CreatedAt: time.Now().UTC().Format(time.RFC3339), Objects: []object{}}
	for entry := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
		if entry.Err != nil {
			return errors.New("object enumeration failed")
		}
		if len(m.Objects) >= 1_000_000 {
			return errors.New("backup object limit exceeded")
		}
		keyHash := sha256.Sum256([]byte(entry.Key))
		name := hex.EncodeToString(keyHash[:]) + ".bin"
		options := minio.GetObjectOptions{}
		if options.SetMatchETag(entry.ETag) != nil {
			return errors.New("invalid source object version")
		}
		source, err := client.GetObject(ctx, bucket, entry.Key, options)
		if err != nil {
			return errors.New("object read failed")
		}
		info, err := source.Stat()
		if err != nil {
			source.Close()
			return errors.New("object metadata read failed")
		}
		file, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			source.Close()
			return errors.New("backup file creation failed")
		}
		hash := sha256.New()
		size, copyErr := io.Copy(io.MultiWriter(file, hash), source)
		closeErr := file.Close()
		source.Close()
		if copyErr != nil || closeErr != nil || size != info.Size {
			return errors.New("backup object changed or copy failed")
		}
		m.Objects = append(m.Objects, object{Key: entry.Key, File: name, SHA256: hex.EncodeToString(hash.Sum(nil)), Size: size, ContentType: info.ContentType})
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil || len(data) >= 64<<20 || ctx.Err() != nil || os.WriteFile(filepath.Join(dir, "manifest.json"), append(data, '\n'), 0600) != nil {
		return errors.New("manifest creation failed")
	}
	fmt.Printf("backed up %d current objects\n", len(m.Objects))
	return nil
}

var filePattern = regexp.MustCompile(`^[a-f0-9]{64}\.bin$`)
var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// readManifest проверяет локальные пути, уникальность объектов и каждую контрольную сумму до восстановления.
// @args dir — приватный каталог готовой резервной копии.
// @return проверенный манифест либо ошибку целостности.
func readManifest(dir string) (manifest, error) {
	var m manifest
	file, err := os.Open(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, errors.New("backup manifest is missing")
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 64<<20))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&m) != nil || decoder.Decode(new(any)) != io.EOF || m.SchemaVersion != 1 || m.Bucket == "" || len(m.Objects) > 1_000_000 {
		return m, errors.New("invalid backup manifest")
	}
	keys, files := map[string]bool{}, map[string]bool{}
	for _, entry := range m.Objects {
		if entry.Key == "" || keys[entry.Key] || files[entry.File] || !filePattern.MatchString(entry.File) || !hashPattern.MatchString(entry.SHA256) || entry.Size < 0 {
			return m, errors.New("invalid backup entry")
		}
		keys[entry.Key], files[entry.File] = true, true
		local := filepath.Join(dir, entry.File)
		info, err := os.Lstat(local)
		if err != nil || !info.Mode().IsRegular() || info.Size() != entry.Size {
			return m, errors.New("backup file is missing, linked or has wrong size")
		}
		f, err := os.Open(local)
		if err != nil {
			return m, errors.New("backup file unreadable")
		}
		hash := sha256.New()
		_, err = io.Copy(hash, f)
		f.Close()
		if err != nil || hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
			return m, errors.New("backup checksum mismatch")
		}
	}
	return m, nil
}

// restore загружает проверенные объекты только в новый или пустой приватный бакет.
// @args ctx — срок операции; client — целевое хранилище; bucket — пустая цель; dir и m — проверенная копия.
// @return ошибку без удаления или перезаписи существующих данных.
func restore(ctx context.Context, client *minio.Client, bucket, dir string, m manifest) error {
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return errors.New("restore bucket check failed")
	}
	if !exists {
		if client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}) != nil {
			return errors.New("restore bucket creation failed")
		}
	}
	for range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true, WithVersions: true}) {
		return errors.New("restore target must be empty and readable, including object versions")
	}
	policy, err := client.GetBucketPolicy(ctx, bucket)
	if err != nil || policy != "" {
		return errors.New("restore target must have no public bucket policy")
	}
	for _, entry := range m.Objects {
		options := minio.PutObjectOptions{ContentType: entry.ContentType}
		options.SetMatchETagExcept("*")
		if _, err := client.FPutObject(ctx, bucket, entry.Key, filepath.Join(dir, entry.File), options); err != nil {
			return errors.New("restore upload failed; target retained for diagnosis")
		}
	}
	return nil
}

// verify повторно читает восстановленные данные и проверяет ограниченный подписанный доступ.
// @args ctx — срок операции; client — S3-клиент; bucket — восстановленный бакет; m — ожидаемый состав.
// @return ошибку при несовпадении байтов или нарушении приватности.
func verify(ctx context.Context, client *minio.Client, bucket string, m manifest) error {
	policy, err := client.GetBucketPolicy(ctx, bucket)
	if err != nil || policy != "" {
		return errors.New("restored bucket privacy check failed")
	}
	for _, entry := range m.Objects {
		stream, err := client.GetObject(ctx, bucket, entry.Key, minio.GetObjectOptions{})
		if err != nil {
			return errors.New("restored object read failed")
		}
		hash := sha256.New()
		size, err := io.Copy(hash, stream)
		stream.Close()
		if err != nil || size != entry.Size || hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
			return errors.New("restored object checksum mismatch")
		}
	}
	if len(m.Objects) > 0 {
		u, err := client.PresignedGetObject(ctx, bucket, m.Objects[0].Key, time.Minute, nil)
		if err != nil {
			return errors.New("restored object signing failed")
		}
		httpClient := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		for _, signed := range []bool{true, false} {
			if !signed {
				u.RawQuery = ""
			}
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			if err != nil {
				return errors.New("restore access request failed")
			}
			response, err := httpClient.Do(request)
			if err != nil {
				return errors.New("restore access check failed")
			}
			io.Copy(io.Discard, io.LimitReader(response.Body, 8192))
			response.Body.Close()
			if (signed && response.StatusCode != http.StatusOK) || (!signed && response.StatusCode != http.StatusForbidden) {
				return errors.New("signed or anonymous access policy failed")
			}
		}
	}
	fmt.Printf("verified %d restored objects; checksums and bucket policy passed; signed/anonymous access checked when non-empty\n", len(m.Objects))
	return nil
}

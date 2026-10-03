package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	chatapp "github.com/janickiy/go-recorder/internal/app/chat"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	s3storage "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	chatusecase "github.com/janickiy/go-recorder/internal/usecase/chat"
)

// TestStageFiveChatFilesRead проверяет сценарий «этап пять чат файлы чтение», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFiveChatFilesRead(t *testing.T) {
	if os.Getenv("RECORDER_STAGE5_CHAT_E2E") != "true" {
		t.Skip("set RECORDER_STAGE5_CHAT_E2E=true with local PostgreSQL, Redis and MinIO")
	}
	f := stageTwo(t)
	ctx := context.Background()
	endpoint, access, secret := os.Getenv("RECORDER_STAGE4_TEST_MINIO_ENDPOINT"), os.Getenv("RECORDER_STAGE4_TEST_MINIO_ACCESS_KEY"), os.Getenv("RECORDER_STAGE4_TEST_MINIO_SECRET_KEY")
	if endpoint == "" {
		endpoint = "localhost:9000"
	}
	if access == "" {
		access = "go_recorder"
	}
	if secret == "" {
		secret = "go_recorder_pass"
	}
	storage, err := s3storage.NewClient(ctx, endpoint, access, secret, "recordings", false)
	if err != nil {
		t.Fatal(err)
	}
	storage.SetPublicEndpoint(endpoint)
	t.Cleanup( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() {
			_ = storage.RemovePrefix(context.Background(), "attachments/"+f.conference.ID+"/")
			for _, user := range []string{f.owner.ID, f.member.ID} {
				keys, _ := f.redis.Keys(context.Background(), "rate:chat_*:"+user+":*").Result()
				if len(keys) > 0 {
					_ = f.redis.Del(context.Background(), keys...).Err()
				}
			}
		})
	repo := pg.NewChatRepository(f.db)
	service, err := chatusecase.NewService(ctx, repo, storage, f.hubs[0])
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	httptransport.RegisterChatRoutes(router, chatapp.NewHandler(service), httpmiddleware.Authenticate(f.tokens), redisinfra.NewRateLimiter(f.redis))
	api := stageOneAPI{router: router}
	path := "/conferences/" + f.conference.ID
	ownerSocket := f.connect(t, 0, f.ownerToken, f.conference.ID)
	memberSocket := f.connect(t, 1, f.memberToken, f.conference.ID)
	api.expect(t, "GET", path+"/messages", "", nil, 401, nil)
	var envelope struct{ Item chat.Message }
	request := chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "Первое сообщение"}
	privateResponse := api.expect(t, "POST", path+"/messages", f.ownerToken, request, 201, &envelope)
	if privateResponse.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("private chat response is cacheable")
	}
	first := envelope.Item
	if first.SenderID != f.owner.ID || first.SenderName != "Alice" || first.Sequence < 1 || first.Version != 1 {
		t.Fatalf("incorrect message: %+v", first)
	}
	memberSocket.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - e (realtime.Envelope): значение e типа realtime.Envelope, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e realtime.Envelope) bool {
			return e.Type == "chat.message.created" && bytes.Contains(e.Data, []byte(first.ID))
		})
	api.expect(t, "POST", path+"/messages", f.ownerToken, request, 200, &envelope)
	if envelope.Item.ID != first.ID {
		t.Fatal("retry duplicated message")
	}
	request.Text = "different"
	api.expect(t, "POST", path+"/messages", f.ownerToken, request, 409, nil)
	api.expect(t, "POST", path+"/messages", f.ownerToken, map[string]any{"clientRequestId": uuid.NewString(), "text": ""}, 422, nil)
	api.expect(t, "POST", path+"/messages", f.ownerToken, map[string]any{"clientRequestId": uuid.NewString(), "text": "x", "senderId": f.member.ID}, 400, nil)
	api.expect(t, "POST", path+"/messages", f.memberToken, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "Ответ", ReplyTo: first.ID}, 201, &envelope)
	reply := envelope.Item
	if reply.ReplyPreview == nil || reply.ReplyPreview.ID != first.ID || reply.ReplyPreview.Text != first.Text {
		t.Fatal("reply preview missing")
	}
	api.expect(t, "PATCH", path+"/messages/"+first.ID, f.memberToken, chat.EditRequest{Text: "forged"}, 403, nil)
	api.expect(t, "PATCH", path+"/messages/"+first.ID, f.ownerToken, chat.EditRequest{Text: "Исправлено"}, 200, &envelope)
	if envelope.Item.Version != 2 {
		t.Fatal("version did not advance")
	}
	ownerSocket.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - e (realtime.Envelope): значение e типа realtime.Envelope, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e realtime.Envelope) bool {
			return e.Type == "chat.message.updated" && bytes.Contains(e.Data, []byte(first.ID))
		})
	var page chat.Page
	api.expect(t, "GET", path+"/messages?limit=1", f.memberToken, nil, 200, &page)
	if len(page.Items) != 1 || page.Items[0].ID != reply.ID || page.NextCursor == "" || page.UnreadCount != 1 {
		t.Fatalf("wrong newest page %+v", page)
	}
	api.expect(t, "GET", path+"/messages?limit=1&before="+url.QueryEscape(page.NextCursor), f.memberToken, nil, 200, &page)
	if len(page.Items) != 1 || page.Items[0].ID != first.ID {
		t.Fatal("cursor skipped original")
	}
	api.expect(t, "GET", path+"/messages?before=malformed", f.memberToken, nil, 422, nil)
	api.expect(t, "GET", path+"/messages?limit=101", f.memberToken, nil, 422, nil)
	var read struct{ Item chat.ReadState }
	api.expect(t, "PUT", path+"/chat/read", f.memberToken, chat.ReadRequest{MessageID: reply.ID}, 200, &read)
	if read.Item.UnreadCount != 0 || read.Item.LastReadMessageID == nil || *read.Item.LastReadMessageID != reply.ID {
		t.Fatal("read cursor not stored")
	}
	memberSocket.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - e (realtime.Envelope): значение e типа realtime.Envelope, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e realtime.Envelope) bool { return e.Type == "chat.read.updated" })
	api.expect(t, "PUT", path+"/chat/read", f.memberToken, chat.ReadRequest{MessageID: first.ID}, 200, &read)
	if *read.Item.LastReadMessageID != reply.ID {
		t.Fatal("read cursor moved backwards")
	}

	t.Run("concurrent retry edit delete and read", /* Вложенный обработчик выполняет отдельный вариант тестового сценария с проверкой результата и очисткой ресурсов.

		@args
		  - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
		*/func(t *testing.T) {
			request := chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "Один запрос"}
			var ids [12]string
			var failures [12]error
			runConcurrent(12, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

				@args
				  - i (int): значение i типа int, используемое согласно назначению этой операции.
				*/func(i int) {
					message, _, err := service.Send(ctx, f.owner.ID, f.conference.ID, request)
					ids[i] = message.ID
					failures[i] = err
				})
			for i, err := range failures {
				if err != nil || ids[i] != ids[0] {
					t.Fatalf("retry %d: %s %v", i, ids[i], err)
				}
			}
			runConcurrent(12, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

				@args
				  - i (int): значение i типа int, используемое согласно назначению этой операции.
				*/func(i int) {
					target := first.ID
					if i%2 == 0 {
						target = ids[0]
					}
					_, failures[i] = service.MarkRead(ctx, f.member.ID, f.conference.ID, chat.ReadRequest{MessageID: target})
				})
			for _, err := range failures {
				if err != nil {
					t.Fatal(err)
				}
			}
			state, err := service.ReadState(ctx, f.member.ID, f.conference.ID)
			if err != nil || state.LastReadMessageID == nil || *state.LastReadMessageID != ids[0] {
				t.Fatal("read race regressed", state, err)
			}
			runConcurrent(12, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

				@args
				  - i (int): значение i типа int, используемое согласно назначению этой операции.
				*/func(i int) {
					if i%2 == 0 {
						_, failures[i] = service.Edit(ctx, f.owner.ID, f.conference.ID, ids[0], chat.EditRequest{Text: "edited"})
					} else {
						_, failures[i] = service.Delete(ctx, f.owner.ID, f.conference.ID, ids[0])
					}
				})
			for _, err := range failures {
				if err != nil && !errors.Is(err, apperrors.ErrConflict) {
					t.Fatal(err)
				}
			}
			var message chat.Message
			if err := f.db.Where("id=?", ids[0]).Take(&message).Error; err != nil || message.DeletedAt == nil || message.Text != "" {
				t.Fatal("edit resurrected deleted message", err)
			}
		})

	data := []byte("Вложение для конференции\n")
	init := chat.InitRequest{ClientRequestID: uuid.NewString(), Filename: "заметки.txt", MimeType: "text/plain", Size: int64(len(data))}
	var upload struct {
		Item      chat.Attachment
		UploadURL string `json:"uploadUrl"`
	}
	api.expect(t, "POST", path+"/attachments/init", f.ownerToken, init, 201, &upload)
	attachment := upload.Item
	api.expect(t, "POST", path+"/attachments/init", f.ownerToken, init, 200, &upload)
	if upload.Item.ID != attachment.ID {
		t.Fatal("attachment init retry duplicated ID")
	}
	uploadPath := path + "/attachments/" + attachment.ID
	api.expect(t, "POST", uploadPath+"/finalize", f.ownerToken, nil, 409, nil)
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	// @args
	//   - token (string): подписанный токен или токен владения, который необходимо проверить.
	//   - payload ([]byte): типизированная нагрузка события или ссылочные сведения уведомления.
	//   - status (int): состояние ресурса, ответа или фильтра выборки.
	rawUpload := func(token string, payload []byte, status int) {
		t.Helper()
		request := httptest.NewRequest("PUT", "/api/v1"+uploadPath+"/content", bytes.NewReader(payload))
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != status {
			t.Fatalf("raw upload got %d want %d: %s", response.Code, status, response.Body.String())
		}
	}
	rawUpload(f.memberToken, data, 403)
	rawUpload(f.ownerToken, []byte("wrong size"), 422)
	rawUpload(f.ownerToken, data, 200)
	rawUpload(f.ownerToken, data, 200)
	api.expect(t, "POST", uploadPath+"/finalize", f.ownerToken, nil, 200, &upload)
	if upload.Item.Status != "ready" || len(upload.Item.Checksum) != 64 {
		t.Fatal("upload not finalized", upload.Item)
	}
	api.expect(t, "POST", uploadPath+"/finalize", f.ownerToken, nil, 200, nil)
	api.expect(t, "POST", path+"/messages", f.memberToken, chat.SendRequest{ClientRequestID: uuid.NewString(), AttachmentIDs: []string{attachment.ID}}, 409, nil)
	attachRequest := chat.SendRequest{ClientRequestID: uuid.NewString(), AttachmentIDs: []string{attachment.ID}}
	api.expect(t, "POST", path+"/messages", f.ownerToken, attachRequest, 201, &envelope)
	fileMessage := envelope.Item
	if len(fileMessage.Attachments) != 1 || fileMessage.Attachments[0].ID != attachment.ID || fileMessage.Text != "" {
		t.Fatal("attachment-only message missing", fileMessage)
	}
	response := api.expect(t, "POST", path+"/messages", f.ownerToken, attachRequest, 200, nil)
	if bytes.Contains(response.Body.Bytes(), []byte("objectKey")) || bytes.Contains(response.Body.Bytes(), []byte("uploadToken")) {
		t.Fatal("private storage metadata leaked")
	}
	var download struct {
		URL       string    `json:"url"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	api.expect(t, "GET", uploadPath+"/download", f.memberToken, nil, 200, &download)
	downloaded, err := http.Get(download.URL)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(downloaded.Body)
	downloaded.Body.Close()
	if err != nil || downloaded.StatusCode != 200 || !bytes.Equal(actual, data) || !strings.HasPrefix(downloaded.Header.Get("Content-Disposition"), "attachment") {
		t.Fatal("authorized attachment download failed", err)
	}
	signed, _ := url.Parse(download.URL)
	if signed.Query().Get("X-Amz-Expires") != "300" {
		t.Fatal("signed URL expiry is not bounded")
	}
	signed.RawQuery = ""
	anonymous, err := http.Get(signed.String())
	if err != nil {
		t.Fatal(err)
	}
	anonymous.Body.Close()
	if anonymous.StatusCode != 403 {
		t.Fatalf("private object accessible anonymously: %d", anonymous.StatusCode)
	}

	t.Run("access scope and admission", /* Вложенный обработчик выполняет отдельный вариант тестового сценария с проверкой результата и очисткой ресурсов.

		@args
		  - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
		*/func(t *testing.T) {
			outsider := users.User{ID: uuid.NewString(), Email: uuid.NewString() + "@stage5.test", PasswordHash: f.owner.PasswordHash}
			if err := f.db.Create(&outsider).Error; err != nil {
				t.Fatal(err)
			}
			token, _ := f.tokens.Issue(outsider.ID)
			for _, suffix := range []string{"/messages", "/chat/read", "/attachments/" + attachment.ID + "/download"} {
				api.expect(t, "GET", path+suffix, token, nil, 403, nil)
			}
			for _, admission := range []string{"waiting", "rejected", "kicked"} {
				if err := f.db.Model(&conferences.Participant{}).Where("conference_id=? AND user_id=?", f.conference.ID, f.member.ID).Updates(map[string]any{"status": admission, "admission_state": admission, "joined_at": nil, "left_at": nil}).Error; err != nil {
					t.Fatal(err)
				}
				api.expect(t, "GET", path+"/messages", f.memberToken, nil, 403, nil)
				api.expect(t, "GET", uploadPath+"/download", f.memberToken, nil, 403, nil)
				api.expect(t, "POST", path+"/messages", f.memberToken, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "bypass"}, 403, nil)
			}
			if err := f.db.Model(&conferences.Participant{}).Where("conference_id=? AND user_id=?", f.conference.ID, f.member.ID).Updates(map[string]any{"status": "joined", "admission_state": "admitted", "joined_at": time.Now().UTC(), "left_at": nil}).Error; err != nil {
				t.Fatal(err)
			}
			other, err := f.service.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "Other chat"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.Join(ctx, f.owner.ID, other.ID, conferences.JoinRequest{}); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.Transition(ctx, f.owner.ID, other.ID, conferences.Active); err != nil {
				t.Fatal(err)
			}
			_, _, err = service.Send(ctx, f.owner.ID, other.ID, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "cross reply", ReplyTo: first.ID})
			if !errors.Is(err, apperrors.ErrInvalidInput) {
				t.Fatal("cross-conference reply accepted", err)
			}
			_, err = service.MarkRead(ctx, f.owner.ID, other.ID, chat.ReadRequest{MessageID: first.ID})
			if !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatal("cross-conference read cursor accepted", err)
			}
			_, _, err = service.Download(ctx, f.owner.ID, other.ID, attachment.ID)
			if !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatal("cross-conference attachment accepted", err)
			}
		})

	t.Run("orphan cleanup and attachment finalize race", /* Вложенный обработчик выполняет отдельный вариант тестового сценария с проверкой результата и очисткой ресурсов.

		@args
		  - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
		*/func(t *testing.T) {
			orphan, _, err := service.InitAttachment(ctx, f.owner.ID, f.conference.ID, chat.InitRequest{ClientRequestID: uuid.NewString(), Filename: "orphan.txt", Size: int64(len(data))})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Upload(ctx, f.owner.ID, f.conference.ID, orphan.ID, bytes.NewReader(data)); err != nil {
				t.Fatal(err)
			}
			if err := f.db.Model(&chat.Attachment{}).Where("id=?", orphan.ID).Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
				t.Fatal(err)
			}
			var results [2]error
			runConcurrent(2, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

				@args
				  - i (int): значение i типа int, используемое согласно назначению этой операции.
				*/func(i int) {
					if i == 0 {
						_, results[i] = service.FinalizeAttachment(ctx, f.owner.ID, f.conference.ID, orphan.ID)
					} else {
						results[i] = service.Cleanup(ctx)
					}
				})
			if !errors.Is(results[0], apperrors.ErrConflict) || results[1] != nil {
				t.Fatal("cleanup/finalize race", results)
			}
			if err := f.db.Where("id=?", orphan.ID).Take(&orphan).Error; err != nil {
				t.Fatal(err)
			}
			if orphan.Status != "expired" || orphan.CleanedAt == nil {
				t.Fatal("orphan was not expired and cleaned")
			}
			if _, _, _, err := storage.StatAttachment(ctx, orphan.ObjectKey); err == nil {
				t.Fatal("expired orphan object still present")
			}
			// Очистка привязанной записи сохраняет именно тот неизменный объект, который выиграл гонку.
			if err := f.db.Model(&chat.Attachment{}).Where("id=?", attachment.ID).Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
				t.Fatal(err)
			}
			if err := service.Cleanup(ctx); err != nil {
				t.Fatal(err)
			}
			if _, _, err := service.Download(ctx, f.member.ID, f.conference.ID, attachment.ID); err != nil {
				t.Fatal("cleanup removed attached object", err)
			}
		})

	t.Run("upload size bound and finalize idempotency", /* Вложенный обработчик выполняет отдельный вариант тестового сценария с проверкой результата и очисткой ресурсов.

		@args
		  - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
		*/func(t *testing.T) {
			oversized, _, err := service.InitAttachment(ctx, f.owner.ID, f.conference.ID, chat.InitRequest{ClientRequestID: uuid.NewString(), Filename: "oversized.txt", Size: chat.MaxAttachmentBytes})
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Upload(ctx, f.owner.ID, f.conference.ID, oversized.ID, io.LimitReader(repeatedChatByte{}, chat.MaxAttachmentBytes+1))
			if !errors.Is(err, apperrors.ErrInvalidInput) {
				t.Fatal("oversized stream accepted", err)
			}
			spoofed, _, err := service.InitAttachment(ctx, f.owner.ID, f.conference.ID, chat.InitRequest{ClientRequestID: uuid.NewString(), Filename: "fake.png", Size: int64(len(data))})
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Upload(ctx, f.owner.ID, f.conference.ID, spoofed.ID, bytes.NewReader(data))
			if !errors.Is(err, apperrors.ErrInvalidInput) {
				t.Fatal("browser MIME spoof accepted", err)
			}
			concurrent, _, err := service.InitAttachment(ctx, f.owner.ID, f.conference.ID, chat.InitRequest{ClientRequestID: uuid.NewString(), Filename: "concurrent.txt", Size: int64(len(data))})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Upload(ctx, f.owner.ID, f.conference.ID, concurrent.ID, bytes.NewReader(data)); err != nil {
				t.Fatal(err)
			}
			var results [12]error
			runConcurrent(12, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

				@args
				  - i (int): значение i типа int, используемое согласно назначению этой операции.
				*/func(i int) {
					_, results[i] = service.FinalizeAttachment(ctx, f.owner.ID, f.conference.ID, concurrent.ID)
				})
			for _, err := range results {
				if err != nil {
					t.Fatal("finalize retry not idempotent", err)
				}
			}
		})

	t.Run("seeded cursor explain and independent rate limit", /* Вложенный обработчик выполняет отдельный вариант тестового сценария с проверкой результата и очисткой ресурсов.

		@args
		  - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
		*/func(t *testing.T) {
			if err := f.db.Exec(`INSERT INTO chat_messages(id,conference_id,sender_user_id,client_request_id,request_fingerprint,text) SELECT gen_random_uuid(),?,?,gen_random_uuid(),repeat('a',64),'seeded' FROM generate_series(1,3000)`, f.conference.ID, f.owner.ID).Error; err != nil {
				t.Fatal(err)
			}
			if err := f.db.Exec(`WITH other AS (INSERT INTO conferences(id,owner_id,title,invite_code,status) SELECT gen_random_uuid(),?,'isolated explain seed',replace(gen_random_uuid()::text,'-',''),'created' FROM generate_series(1,12) RETURNING id) INSERT INTO chat_messages(id,conference_id,sender_user_id,client_request_id,request_fingerprint,text) SELECT gen_random_uuid(),other.id,?,gen_random_uuid(),repeat('b',64),'other conference' FROM other CROSS JOIN generate_series(1,500)`, f.owner.ID, f.owner.ID).Error; err != nil {
				t.Fatal(err)
			}
			if err := f.db.Exec("ANALYZE chat_messages").Error; err != nil {
				t.Fatal(err)
			}
			rows, err := f.db.Raw(`EXPLAIN (ANALYZE,BUFFERS) SELECT id,sequence,text FROM chat_messages WHERE conference_id=? AND sequence<99999999 ORDER BY sequence DESC LIMIT 50`, f.conference.ID).Rows()
			if err != nil {
				t.Fatal(err)
			}
			var plan []string
			for rows.Next() {
				var line string
				if err := rows.Scan(&line); err != nil {
					t.Fatal(err)
				}
				plan = append(plan, line)
			}
			rows.Close()
			t.Log("cursor EXPLAIN:", strings.Join(plan, "\n"))
			if !strings.Contains(strings.Join(plan, " "), "chat_messages_conference_sequence") || strings.Contains(strings.Join(plan, " "), "Rows Removed by Filter") {
				t.Fatal("cursor query did not use the conference-scoped index")
			}
			// Отметка прочтения использует отдельный ключ лимита; его исчерпание не блокирует отправку сообщений.
			var latest chat.Message
			if err := f.db.Where("conference_id=?", f.conference.ID).Order("sequence DESC").First(&latest).Error; err != nil {
				t.Fatal(err)
			}
			blocked := false
			for i := 0; i < 35; i++ {
				body, _ := json.Marshal(chat.ReadRequest{MessageID: latest.ID})
				request := httptest.NewRequest("PUT", "/api/v1"+path+"/chat/read", bytes.NewReader(body))
				request.Header.Set("Authorization", "Bearer "+f.ownerToken)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code == 429 {
					blocked = true
					break
				}
				if response.Code != 200 {
					t.Fatalf("read rate response: %d %s", response.Code, response.Body.String())
				}
			}
			if !blocked {
				t.Fatal("read updates lack rate limit")
			}
			api.expect(t, "POST", path+"/messages", f.ownerToken, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "separate budget"}, 201, nil)
		})

	t.Run("bounded HTTP burst and critical snapshot", /* Вложенный обработчик выполняет отдельный вариант тестового сценария с проверкой результата и очисткой ресурсов.

		@args
		  - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
		*/func(t *testing.T) {
			live := f.connect(t, 1, f.memberToken, f.conference.ID)
			const count = 80
			statuses := make([]int, count)
			done := make(chan struct{})
			start := time.Now()
			go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			 */func() {
				defer close(done)
				runConcurrent(count, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

					@args
					  - i (int): значение i типа int, используемое согласно назначению этой операции.
					*/func(i int) {
						body, _ := json.Marshal(chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "bounded burst"})
						request := httptest.NewRequest("POST", "/api/v1"+path+"/messages", bytes.NewReader(body))
						request.Header.Set("Authorization", "Bearer "+f.memberToken)
						response := httptest.NewRecorder()
						router.ServeHTTP(response, request)
						statuses[i] = response.Code
					})
			}()
			f.hubs[0].ConferenceChanged(ctx, f.conference.ID)
			live.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

				@args
				  - event (realtime.Envelope): конверт входящего или публикуемого события.

				@return:
				  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(event realtime.Envelope) bool { return event.Type == "conference.state" })
			latency := time.Since(start)
			<-done
			accepted, limited := 0, 0
			for _, status := range statuses {
				switch status {
				case 201:
					accepted++
				case 429:
					limited++
				default:
					t.Fatalf("unexpected burst response %d", status)
				}
			}
			if accepted == 0 || accepted > 60 || limited == 0 {
				t.Fatalf("unbounded burst accepted=%d limited=%d", accepted, limited)
			}
			api.expect(t, "POST", "/conferences/"+strings.ToUpper(f.conference.ID)+"/messages", f.memberToken, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "UUID-case bypass"}, 429, nil)
			if latency > 2*time.Second {
				t.Fatalf("critical snapshot starved for %s", latency)
			}
			t.Logf("80-request chat burst: accepted=%d rate-limited=%d critical snapshot latency=%s total=%s", accepted, limited, latency, time.Since(start))
		})
	api.expect(t, "DELETE", path+"/messages/"+fileMessage.ID, f.ownerToken, nil, 200, &envelope)
	api.expect(t, "GET", uploadPath+"/download", f.memberToken, nil, 404, nil)
	api.expect(t, "DELETE", path+"/messages/"+first.ID, f.ownerToken, nil, 200, nil)
	api.expect(t, "PATCH", path+"/messages/"+first.ID, f.ownerToken, chat.EditRequest{Text: "resurrect"}, 409, nil)
	if _, err := f.service.Transition(ctx, f.owner.ID, f.conference.ID, conferences.Finished); err != nil {
		t.Fatal(err)
	}
	api.expect(t, "GET", path+"/messages", f.memberToken, nil, 200, nil)
	api.expect(t, "POST", path+"/messages", f.ownerToken, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "closed"}, 409, nil)
	api.expect(t, "PATCH", path+"/messages/"+reply.ID, f.memberToken, chat.EditRequest{Text: "closed"}, 409, nil)
	t.Log("chat persistence/realtime, idempotency, replies, ownership, read races, private uploads/downloads, admission, cleanup and finished read-only passed")
}

// repeatedChatByte хранит изолированное состояние тестового компонента «repeated чат Byte».
type repeatedChatByte struct{}

// Read читает состояние ресурсов компонента для дальнейшей обработки или ответа.
//
// @args
//   - p ([]byte): байты, переданные по контракту io.Writer.
//
// @return:
//   - результат 1 (int): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (repeatedChatByte) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

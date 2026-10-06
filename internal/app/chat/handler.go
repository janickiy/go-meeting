package chatapp

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	chatusecase "github.com/janickiy/go-recorder/internal/usecase/chat"
)

// Handler связывает транспортный запрос с прикладным сценарием, проверкой входных данных и формированием ответа.
//   - service: значение service типа *chatusecase.Service, используемое согласно назначению этой операции.
type Handler struct {
	service   *chatusecase.Service
	namespace string
}

// NewHandler создаёт и связывает зависимости компонента Handler, используемого в постоянном чате и приватных вложениях.
//
// @args
//   - service (*chatusecase.Service): значение service типа *chatusecase.Service, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (*Handler): созданный компонент с переданными зависимостями.
func NewHandler(service *chatusecase.Service) *Handler {
	return &Handler{service: service, namespace: "conferences"}
}

// parameter проверяет обязательный параметр HTTP-маршрута перед прикладной операцией.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//   - key (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (bool): признак выполнения проверяемого условия или изменения состояния.
func parameter(c *gin.Context, key string) (string, bool) {
	id, err := chat.UUID(c.Param(key))
	if err != nil {
		httpresponse.Fail(c, err)
		return "", false
	}
	return id, true
}

// List возвращает ограниченный список сообщений и вложений конференции с принятыми в данном слое фильтрами.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) List(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	limit := 50
	if raw, exists := c.GetQuery("limit"); exists {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			httpresponse.Fail(c, apperrors.New(apperrors.ErrInvalidInput, "limit must be between 1 and 100"))
			return
		}
	}
	page, err := h.service.List(c.Request.Context(), httpmiddleware.UserID(c), id, c.Query("before"), limit)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "nextCursor": page.NextCursor, "unreadCount": page.UnreadCount, "lastReadMessageId": page.LastReadMessageID})
}

// Send сохраняет сообщение чата с проверкой доступа, ответа и вложений; ключ запроса защищает повторную отправку от дубля.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Send(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	var request chat.SendRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, created, err := h.service.Send(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"status": "success", "item": item})
}

// Edit изменяет текст собственного неудалённого сообщения с проверкой доступа и состояния встречи.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Edit(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	messageID, ok := parameter(c, "messageId")
	if !ok {
		return
	}
	var request chat.EditRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.service.Edit(c.Request.Context(), httpmiddleware.UserID(c), id, messageID, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// Delete мягко удаляет доступное сообщение, проверяя автора или полномочия модератора и сохраняя историю.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	messageID, ok := parameter(c, "messageId")
	if !ok {
		return
	}
	item, err := h.service.Delete(c.Request.Context(), httpmiddleware.UserID(c), id, messageID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// ReadState возвращает сохранённую границу прочтения и число доступных непрочитанных сообщений.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) ReadState(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	item, err := h.service.ReadState(c.Request.Context(), httpmiddleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// MarkRead продвигает сохранённое состояние прочтения; повторные и запоздалые запросы не должны уменьшать курсор.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) MarkRead(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	var request chat.ReadRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.service.MarkRead(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// InitAttachment создаёт или возвращает метаданные незавершённого вложения для безопасной повторной загрузки.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) InitAttachment(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	var request chat.InitRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, created, err := h.service.InitAttachment(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"status": "success", "item": item, "uploadUrl": "/api/v1/" + h.namespace + "/" + id + "/attachments/" + item.ID + "/content"})
}

// Upload принимает ограниченное тело загрузки, проверяет содержимое и сохраняет объект действующей попытки.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Upload(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	attachmentID, ok := parameter(c, "attachmentId")
	if !ok {
		return
	}
	// Ограничиваем медленных клиентов и объём данных. Загрузки отделены от очередей сигнализации
	// и модерации; сервис ограничивает число одновременных загрузок в памяти.
	controller := http.NewResponseController(c.Writer)
	_ = controller.SetReadDeadline(time.Now().Add(60 * time.Second))
	defer controller.SetReadDeadline(time.Time{})
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, chat.MaxAttachmentBytes+1)
	item, err := h.service.Upload(c.Request.Context(), httpmiddleware.UserID(c), id, attachmentID, c.Request.Body)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// FinalizeAttachment подтверждает готовность загруженного вложения к привязке к сообщению.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) FinalizeAttachment(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	attachmentID, ok := parameter(c, "attachmentId")
	if !ok {
		return
	}
	if !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	item, err := h.service.FinalizeAttachment(c.Request.Context(), httpmiddleware.UserID(c), id, attachmentID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// Download проверяет доступ к прикреплённому файлу и выдаёт временную подписанную ссылку чтения.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Download(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	attachmentID, ok := parameter(c, "attachmentId")
	if !ok {
		return
	}
	url, expires, err := h.service.Download(c.Request.Context(), httpmiddleware.UserID(c), id, attachmentID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"status": "success", "url": url, "expiresAt": expires})
}

// ForConversations preserves the shared HTTP/file flow with a distinct public scope.
func (h *Handler) ForConversations() *Handler { h.namespace = "conversations"; return h }

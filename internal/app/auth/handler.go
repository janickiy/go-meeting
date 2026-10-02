package authapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/users"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

// Service задаёт контракт зависимого компонента Service в авторизации и учётных записях пользователей; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//
//	@params:
//
// - Register: операция Register с контрактом, описанным у метода.
// - Login: операция Login с контрактом, описанным у метода.
// - Me: операция Me с контрактом, описанным у метода.
type Service interface {
	// Register проверяет данные регистрации, хеширует пароль и создаёт новую учётную запись.
	//
	// @parameters:
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (users.RegisterRequest): входные параметры соответствующего прикладного запроса.
	//
	// @return:
	//   - результат 1 (users.View): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Register(context.Context, users.RegisterRequest) (users.View, error)
	// Login проверяет учётные данные и возвращает безопасные сведения пользователя и токен входа.
	//
	// @parameters:
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (users.LoginRequest): входные параметры соответствующего прикладного запроса.
	//
	// @return:
	//   - результат 1 (users.LoginResponse): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Login(context.Context, users.LoginRequest) (users.LoginResponse, error)
	// Me читает публичные сведения текущего авторизованного пользователя.
	//
	// @parameters:
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//
	// @return:
	//   - результат 1 (users.View): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Me(context.Context, string) (users.View, error)
}

// Handler связывает транспортный запрос с прикладным сценарием, проверкой входных данных и формированием ответа.
//   - service: значение service типа Service, используемое согласно назначению этой операции.
type Handler struct{ service Service }

// NewHandler создаёт и связывает зависимости компонента Handler, используемого в авторизации и учётных записях пользователей.
//
// @parameters:
//   - service (Service): значение service типа Service, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (*Handler): созданный компонент с переданными зависимостями.
func NewHandler(service Service) *Handler { return &Handler{service: service} }

// Register проверяет данные регистрации, хеширует пароль и создаёт новую учётную запись.
//
// @parameters:
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Register(c *gin.Context) {
	var request users.RegisterRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	user, err := h.service.Register(c.Request.Context(), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "success", "user": user})
}

// Login проверяет учётные данные и возвращает безопасные сведения пользователя и токен входа.
//
// @parameters:
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Login(c *gin.Context) {
	var request users.LoginRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	response, err := h.service.Login(c.Request.Context(), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

// Me читает публичные сведения текущего авторизованного пользователя.
//
// @parameters:
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Me(c *gin.Context) {
	user, err := h.service.Me(c.Request.Context(), httpmiddleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "user": user})
}

// Logout обрабатывает завершение клиентской авторизации по принятому API-контракту.
//
// @parameters:
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Logout(c *gin.Context) {
	if !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Discard the access token on the client; the token remains valid until expiration"})
}

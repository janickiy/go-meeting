package httptransport

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// registerRetiredRecordingRoutes закрывает старый API записи и его отладочные
// обходы для всех клиентов. У старых записей нет проверяемой модели владельца,
// поэтому наличие JWT само по себе не даёт права читать или изменять эти данные.
// Обработчики не обращаются к базе, хранилищу и воркеру; включить их через
// окружение, локальный режим или альтернативный префикс нельзя.
//
// @args
//   - router: маршрутизатор, в котором регистрируются ответы об удалённом API.
func registerRetiredRecordingRoutes(router gin.IRouter) {
	for _, prefix := range []string{APIV1Prefix + "/records", "/api/records", "/debug/records"} {
		router.Any(prefix, retiredRecordingEndpoint)
		router.Any(prefix+"/*path", retiredRecordingEndpoint)
	}
	// Страница WebRTC smoke удалена; прежний адрес остаётся закрытой заглушкой.
	router.Any("/debug/webrtc-smoke", retiredRecordingEndpoint)
}

// retiredRecordingEndpoint сообщает об окончательном закрытии старого API,
// не раскрывая наличие записи, её состояние или подписанные ссылки на файлы.
//
// @args
//   - c: HTTP-запрос; учётные данные и идентификаторы ресурса не используются.
func retiredRecordingEndpoint(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.AbortWithStatusJSON(http.StatusGone, gin.H{
		"status":  "failed",
		"message": "legacy recording API is no longer available; use authenticated conference recording endpoints",
	})
}

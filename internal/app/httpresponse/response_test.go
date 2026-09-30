package httpresponse

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestInternalErrorsDoNotExposeInfrastructureDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/fail", func(c *gin.Context) {
		Fail(c, errors.New("SQL failure: password_hash=private-hash, password=private-password"))
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/fail", nil))
	if response.Code != 500 || response.Body.String() != `{"message":"internal server error","status":"failed"}` {
		t.Fatal("internal error response leaked details or changed contract")
	}
}

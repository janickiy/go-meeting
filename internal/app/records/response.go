package recordsapp

import (
	"git.svc-dev.net/board/go-recorder/internal/domain/records"
	"github.com/gin-gonic/gin"
)

func failed(c *gin.Context, status int, message string) {
	c.JSON(status, records.Response{Status: "failed", Message: message})
}

package recordsapp

import (
	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

func failed(c *gin.Context, status int, message string) {
	c.JSON(status, records.Response{Status: "failed", Message: message})
}

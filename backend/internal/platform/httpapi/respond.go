package httpapi

import (
	"github.com/gin-gonic/gin"
)

// errorBody is the single error format of the API:
// {"error": {"code": "machine_readable", "message": "текст для пользователя"}}.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(c *gin.Context, status int, v any) {
	c.Header("Cache-Control", "no-store")
	c.JSON(status, v)
}

func writeError(c *gin.Context, status int, code, message string) {
	writeJSON(c, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

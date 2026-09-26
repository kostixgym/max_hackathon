package httpapi

import (
	"github.com/gin-gonic/gin"
)

// errorBody is the single error format of the API:
// {"error": {"code": "machine_readable", "message": "текст для пользователя"}}.
// An error about one field of a form also names it: "field": "camera_count".
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func writeJSON(c *gin.Context, status int, v any) {
	c.Header("Cache-Control", "no-store")
	c.JSON(status, v)
}

func writeError(c *gin.Context, status int, code, message string) {
	writeJSON(c, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

func writeFieldError(c *gin.Context, status int, code, message, field string) {
	writeJSON(c, status, errorBody{Error: errorDetail{Code: code, Message: message, Field: field}})
}

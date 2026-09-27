package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
)

// notImplemented answers a routed-but-empty endpoint of the sprint skeleton
// (docs/plan-do-30-09.md, К0): the route, access and error codes are final, the
// handler lands with its module. The front hides such buttons, curl sees 501.
func notImplemented(c *gin.Context, message string) {
	writeError(c, http.StatusNotImplemented, "not_implemented", message)
}

// resourceIDOk reports whether the value can be a UUID from a URL path.
func resourceIDOk(v string) bool {
	var id pgtype.UUID

	return id.Scan(v) == nil && id.Valid
}

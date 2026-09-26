package httpapi

// Document endpoints (Дима, docs/plan-do-30-09.md, Д5): the meeting protocol lives
// here, not in meetings.go, so that Гоша and Дима never edit the same file. The
// data comes from Meetings (Гоша's ProtocolData), the PDF from the pure functions
// of the documents module.

import (
	"github.com/gin-gonic/gin"
)

func (h *handlers) meetingProtocolPDF(c *gin.Context) {
	notImplemented(c, "Протокол появится вместе с модулем документов (Д5)")
}

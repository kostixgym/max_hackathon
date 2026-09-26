package httpapi

// Meeting endpoints (Гоша, docs/plan-do-30-09.md, Г2–Г6). The routes and error
// codes are fixed by this skeleton: replace the 501 stubs, add the Meetings
// interface to Deps in server.go (Костя) and the wiring in main.go.

import (
	"github.com/gin-gonic/gin"
)

func (h *handlers) createMeeting(c *gin.Context) {
	notImplemented(c, "Собрание появится вместе с модулем собрания (Г2)")
}

func (h *handlers) getMeeting(c *gin.Context) {
	notImplemented(c, "Собрание появится вместе с модулем собрания (Г3)")
}

func (h *handlers) meetingTracker(c *gin.Context) {
	notImplemented(c, "Трекер появится вместе с модулем собрания (Г3)")
}

func (h *handlers) receiveBallot(c *gin.Context) {
	notImplemented(c, "Отметка бюллетеней появится вместе с модулем собрания (Г3)")
}

func (h *handlers) ballotDecisions(c *gin.Context) {
	notImplemented(c, "Внесение решений появится вместе с модулем собрания (Г4)")
}

func (h *handlers) meetingResultPreview(c *gin.Context) {
	notImplemented(c, "Предпросмотр итога появится вместе с модулем собрания (Г5)")
}

func (h *handlers) finalizeMeeting(c *gin.Context) {
	notImplemented(c, "Фиксация итога появится вместе с модулем собрания (Г5)")
}

func (h *handlers) meetingProtocolPDF(c *gin.Context) {
	notImplemented(c, "Протокол появится вместе с модулем документов (Д5)")
}

func (h *handlers) demoFinishVoting(c *gin.Context) {
	notImplemented(c, "Демо-ускорители появятся вместе с модулем собрания (Г6)")
}

func (h *handlers) demoFillBallots(c *gin.Context) {
	notImplemented(c, "Демо-ускорители появятся вместе с модулем собрания (Г6)")
}

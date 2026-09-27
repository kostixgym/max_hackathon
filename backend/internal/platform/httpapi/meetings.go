package httpapi

// Meeting endpoints (Гоша, docs/plan-do-30-09.md, Г2–Г6). The routes, the Deps
// field and the error codes are fixed by the skeleton: replace the 501 stubs, add
// methods to Meetings below and the wiring in main.go. server.go needs no change.
// The staff acts through access.ManagesAsStaff, not IsStaffOf: in the demo house a
// tester runs only the meetings of their own initiatives (решение 79). The protocol
// PDF is Дима's (documents.go).

import (
	"github.com/gin-gonic/gin"
)

// Meetings is the meeting module (Гоша adds the methods in Г2, h.meetings is nil
// until the wiring in main.go).
type Meetings interface{}

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

func (h *handlers) demoFinishVoting(c *gin.Context) {
	notImplemented(c, "Демо-ускорители появятся вместе с модулем собрания (Г6)")
}

func (h *handlers) demoFillBallots(c *gin.Context) {
	notImplemented(c, "Демо-ускорители появятся вместе с модулем собрания (Г6)")
}

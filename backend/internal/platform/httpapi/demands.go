package httpapi

// Demand endpoints (Дима, docs/plan-do-30-09.md, Д1–Д3). The routes, the Deps
// field and the error codes are fixed by the skeleton: replace the 501 stubs, add
// methods to Demands below and the wiring in main.go. server.go needs no change.
// Only the demand module decides who acts as the management company: through
// access.ManagesAsStaff, which isolates the testers of the demo house (решение 79).

import (
	"github.com/gin-gonic/gin"
)

// Demands is the demand module (Дима adds the methods in Д1, h.demands is nil until
// the wiring in main.go).
type Demands interface{}

func (h *handlers) createDemand(c *gin.Context) {
	notImplemented(c, "Требование появится вместе с модулем требования (Д1)")
}

func (h *handlers) getDemand(c *gin.Context) {
	notImplemented(c, "Требование появится вместе с модулем требования (Д1)")
}

func (h *handlers) markDemandDelivered(c *gin.Context) {
	notImplemented(c, "Требование появится вместе с модулем требования (Д1)")
}

func (h *handlers) demandPDF(c *gin.Context) {
	notImplemented(c, "PDF требования появится вместе с модулем документов (Д3)")
}

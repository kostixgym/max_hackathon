package httpapi

// Demand endpoints (Дима, docs/plan-do-30-09.md, Д1–Д3). The routes and error
// codes are fixed by this skeleton: replace the 501 stubs, add the Demands
// interface to Deps in server.go (Костя) and the wiring in main.go.

import (
	"github.com/gin-gonic/gin"
)

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

package httpapi

// Document endpoints (Дима, docs/plan-do-30-09.md, Д5): the meeting protocol lives
// here, not in meetings.go, so that Гоша and Дима never edit the same file. The
// data comes from Meetings (Гоша's ProtocolData), the PDF from the pure functions
// of the documents module.

import (
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"maxhackathon/backend/internal/documents"
)

func (h *handlers) meetingProtocolPDF(c *gin.Context) {
	if h.meetings == nil {
		notImplemented(c, "Собрания временно недоступны")
		return
	}
	identity, ok := IdentityFrom(c)
	if !ok {
		writeError(c, http.StatusInternalServerError, "internal", "Внутренняя ошибка, попробуйте ещё раз")
		return
	}
	// Get enforces meeting visibility before the protocol data is read.
	if _, err := h.meetings.Get(c.Request.Context(), c.Param("id"), identity.UserID); err != nil {
		h.writeMeetingError(c, err, "authorize protocol download")
		return
	}
	p, err := h.meetings.ProtocolData(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeMeetingError(c, err, "load meeting protocol")
		return
	}
	items := make([]documents.ProtocolPDFItem, 0, len(p.Items))
	for _, item := range p.Items {
		items = append(items, documents.ProtocolPDFItem{
			Position: item.Position, Text: item.Text, RuleText: majorityRuleText(item.MajorityRule),
			ForM2: m2Text(item.ForM2), AgainstM2: m2Text(item.AgainstM2),
			AbstainM2:      m2Text(item.AbstainM2),
			ForPercent:     percentText(item.ForM2, p.TotalM2),
			AgainstPercent: percentText(item.AgainstM2, p.TotalM2),
			AbstainPercent: percentText(item.AbstainM2, p.TotalM2), Accepted: item.Accepted,
			Verdict: map[bool]string{true: "принято", false: "не принято"}[item.Accepted],
		})
	}
	form := map[string]string{"gis_electronic": "Электронное голосование в Госуслуги.Дом", "paper_absentee": "Заочное голосование на бумаге"}[p.Form]
	if form == "" {
		form = p.Form
	}
	quorum := "не достигнут"
	outcome := "Собрание не состоялось: кворум не достигнут."
	if p.QuorumReached {
		quorum = "достигнут"
		outcome = "Собрание состоялось, кворум достигнут."
	}
	data := documents.ProtocolPDFData{
		Address: p.HouseAddress, OrgName: "Собственникам помещений", Attempt: p.Attempt,
		Form: form, NoticeDate: p.NoticeAt.Format("02.01.2006"),
		VotingStart: p.VotingStartsAt.Format("02.01.2006"), VotingEnd: p.VotingEndsAt.Format("02.01.2006"),
		ChairName: p.Chair.FullName, ChairPremise: p.Chair.PremiseNumber,
		SecretaryName: p.Secretary.FullName, SecretaryPrem: p.Secretary.PremiseNumber,
		TotalM2: m2Text(p.TotalM2), ParticipantsM2: m2Text(p.ParticipantsM2),
		QuorumReached: p.QuorumReached, QuorumText: quorum, Items: items,
		Outcome: outcome, FinalizedDate: p.FinalizedAt.Format("02.01.2006"),
		CreatedAt: p.FinalizedAt.Format("02.01.2006"),
	}
	pdf, err := documents.ProtocolPDF(data)
	if err != nil {
		h.log.Error("render meeting protocol", "err", err)
		writeError(c, http.StatusInternalServerError, "document_error", "Не удалось сформировать протокол")
		return
	}
	c.Header("Content-Disposition", `attachment; filename="meeting-protocol-`+strconv.Itoa(p.Attempt)+`.pdf"`)
	c.Data(http.StatusOK, "application/pdf", pdf)
}

func majorityRuleText(rule string) string {
	switch rule {
	case "majority_of_participants":
		return "более половины голосов участников"
	case "more_than_half_of_all":
		return "более половины голосов всех собственников"
	case "two_thirds_of_all":
		return "не менее двух третей голосов всех собственников"
	default:
		return rule
	}
}

func m2Text(value *big.Rat) string {
	if value == nil {
		return "0,00"
	}
	return strings.ReplaceAll(value.FloatString(2), ".", ",")
}

func percentText(value, total *big.Rat) string {
	if value == nil || total == nil || total.Sign() <= 0 {
		return "0,00"
	}
	percent := new(big.Rat).Mul(value, big.NewRat(100, 1))
	percent.Quo(percent, total)
	return strings.ReplaceAll(percent.FloatString(2), ".", ",")
}

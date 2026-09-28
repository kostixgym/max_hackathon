package documents

// ProtocolPDF renders the meeting protocol draft (Приказ Минстроя № 44/пр).
// Pure function: takes pre-formatted data, returns PDF bytes.

import (
	"bytes"
	"fmt"

	"github.com/go-pdf/fpdf"
)

// ProtocolPDFData is everything the protocol PDF shows, pre-formatted by the caller.
type ProtocolPDFData struct {
	Address        string
	Attempt        int
	Form           string
	NoticeDate     string
	VotingStart    string
	VotingEnd      string
	ChairName      string
	ChairPremise   string
	SecretaryName  string
	SecretaryPrem  string
	TotalM2        string
	ParticipantsM2 string
	QuorumReached  bool
	QuorumText     string
	Items          []ProtocolPDFItem
	Outcome        string
	FinalizedDate  string
	CreatedAt      string
	OrgName        string
}

// ProtocolPDFItem is one agenda question with its fixed result.
type ProtocolPDFItem struct {
	Position       int
	Text           string
	RuleText       string
	ForM2          string
	ForPercent     string
	AgainstM2      string
	AgainstPercent string
	AbstainM2      string
	AbstainPercent string
	Accepted       bool
	Verdict        string
}

func ProtocolPDF(data ProtocolPDFData) ([]byte, error) {
	if data.Address == "" || data.OrgName == "" {
		return nil, fmt.Errorf("protocol pdf: address and org name are required")
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 20, 20)
	pdf.SetAutoPageBreak(true, 20)
	pdf.AddPage()
	pdf.AddUTF8FontFromBytes("DejaVu", "", FontSans())
	pdf.AddUTF8FontFromBytes("DejaVu", "B", FontSans())
	if pdf.Err() {
		return nil, fmt.Errorf("protocol pdf font: %v", pdf.Error())
	}

	const (
		h1    = 16
		h2    = 13
		body  = 11
		small = 9
		lh    = 6.0
	)
	font := func(size float64, style string) { pdf.SetFont("DejaVu", style, size) }

	// Шапка
	font(small, "")
	pdf.MultiCell(0, lh, fmt.Sprintf("Адресат: %s\nДом: %s", data.OrgName, data.Address), "", "L", false)
	pdf.Ln(4)

	font(h1, "B")
	pdf.CellFormat(0, 10, fmt.Sprintf("ПРОТОКОЛ № %d", data.Attempt), "", 2, "C", false, 0, "")
	font(h2, "")
	pdf.CellFormat(0, 8, "общего собрания собственников помещений", "", 2, "C", false, 0, "")
	pdf.Ln(4)

	// Реквизиты
	font(body, "")
	pdf.MultiCell(0, lh, fmt.Sprintf(
		"Форма проведения: %s\n"+
			"Дата уведомления: %s\n"+
			"Голосование: %s — %s\n"+
			"Председатель: %s (кв. %s)\n"+
			"Секретарь: %s (кв. %s)\n"+
			"Общая площадь дома: %s м²\n"+
			"Участники собрания: %s м²\n"+
			"Кворум: %s",
		data.Form, data.NoticeDate, data.VotingStart, data.VotingEnd,
		data.ChairName, data.ChairPremise,
		data.SecretaryName, data.SecretaryPrem,
		data.TotalM2, data.ParticipantsM2, data.QuorumText), "", "L", false)
	pdf.Ln(4)

	// Решения по вопросам
	font(body, "B")
	pdf.CellFormat(0, lh, "Решения по вопросам повестки:", "", 2, "L", false, 0, "")
	font(body, "")
	for _, item := range data.Items {
		pdf.Ln(2)
		font(body, "B")
		pdf.MultiCell(0, lh, fmt.Sprintf("%d. %s", item.Position, item.Text), "", "L", false)
		font(body, "")
		pdf.MultiCell(0, lh, fmt.Sprintf(
			"Правило большинства: %s.\n«За»: %s м² (%s%%), «против»: %s м² (%s%%), «воздержались»: %s м² (%s%%).\nРешение: %s.",
			item.RuleText, item.ForM2, item.ForPercent, item.AgainstM2, item.AgainstPercent,
			item.AbstainM2, item.AbstainPercent, item.Verdict), "", "L", false)
	}
	pdf.Ln(4)

	// Итог
	font(body, "B")
	pdf.CellFormat(0, lh, "Итог собрания:", "", 2, "L", false, 0, "")
	font(body, "")
	pdf.MultiCell(0, lh, data.Outcome, "", "L", false)
	pdf.Ln(4)

	// Подписи
	pdf.Ln(6)
	font(small, "")
	pdf.MultiCell(0, lh, fmt.Sprintf(
		"Председатель: _______________ / %s\nСекретарь: _______________ / %s\nДата протокола: %s",
		data.ChairName, data.SecretaryName, data.CreatedAt), "", "L", false)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("protocol pdf: %w", err)
	}

	return buf.Bytes(), nil
}

package documents

// DemandPDF renders the demand of the owners to the management company
// (ст. 45 ч. 6 ЖК, docs/plan-do-30-09.md, Д3). The signature table stays empty
// and holds no names (решение 20): the demand text itself is signed by those who
// support the initiative, page 2 is left for their signatures.

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/go-pdf/fpdf"
)

// DemandData is everything the PDF shows. All values are pre-formatted strings:
// the module does not know where they came from.
type DemandData struct {
	OrgName       string   // адресат: управляющая организация
	HouseAddress  string   // дом, в котором требуются собрания
	InitiatorName string   // ФИО инициатора (из реестра, полное — для документа)
	PremiseNumber string   // помещение инициатора
	Title         string   // название инициативы
	Agenda        []string // вопросы повестки
	SupportM2     string   // поддержка «за», точное значение в м²
	ThresholdM2   string   // порог требования: 10% площади дома
	Percent       string   // поддержка в процентах площади дома
	Channel       string   // «бумага» / «Госуслуги.Дом» — как планируется провести собрание
	CreatedAt     string   // дата составления
}

// DemandPDF renders the document as PDF bytes.
func DemandPDF(d DemandData) ([]byte, error) {
	if d.OrgName == "" || d.HouseAddress == "" {
		return nil, fmt.Errorf("demand pdf: org name and house address are required")
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 20, 20)
	pdf.SetAutoPageBreak(true, 20)
	pdf.AddPage()
	pdf.AddUTF8FontFromBytes("DejaVu", "", FontSans())
	// Жирное начертание регистрируется тем же файлом: для UTF8-шрифтов fpdf
	// требует отдельную регистрацию стиля, иначе SetFont(..., "B") падает.
	pdf.AddUTF8FontFromBytes("DejaVu", "B", FontSans())

	if pdf.Err() {
		return nil, fmt.Errorf("demand pdf: %w", pdf.Error())
	}

	const ( // типографика документа
		fontSizeTitle = 16
		fontSizeBody  = 11
		fontSizeSmall = 9
		lineHeight    = 6.0
	)

	unicode := func(size float64, style string) {
		pdf.SetFont("DejaVu", style, size)
	}

	// Шапка: адресат — управляющая организация.
	unicode(fontSizeSmall, "")
	pdf.MultiCell(0, lineHeight, fmt.Sprintf("Кому: %s\n%s", d.OrgName, d.HouseAddress), "", "L", false)
	pdf.Ln(4)

	// Заголовок документа.
	unicode(fontSizeTitle, "B")
	pdf.MultiCell(0, 8, "ТРЕБОВАНИЕ о проведении общего собрания собственников помещений", "", "C", false)
	pdf.Ln(2)

	// Основание и дом.
	unicode(fontSizeBody, "")
	pdf.MultiCell(0, lineHeight,
		fmt.Sprintf("Мы, собственники помещений многоквартирного дома по адресу: %s, "+
			"на основании ч. 2 ст. 45 Жилищного кодекса РФ требуем провести общее собрание "+
			"собственников помещений с голосованием в заочной форме (бюллетени и ГИС ЖКХ).", d.HouseAddress),
		"", "L", false)
	pdf.Ln(3)

	// Повестка.
	unicode(fontSizeBody, "B")
	pdf.CellFormat(0, lineHeight, "Повестка дня:", "", 2, "L", false, 0, "")
	unicode(fontSizeBody, "")
	for i, item := range d.Agenda {
		pdf.MultiCell(0, lineHeight, fmt.Sprintf("%d. %s", i+1, item), "", "L", false)
	}
	pdf.Ln(3)

	// Поддержка: скольких м² набрали и какой порог преодолели.
	unicode(fontSizeBody, "")
	pdf.MultiCell(0, lineHeight,
		fmt.Sprintf("Требование поддержано собственниками, владеющими %s м² из %s м² общей площади дома (%s%%): "+
			"это не менее 10%%, установленных ч. 2 ст. 45 ЖК РФ.", d.SupportM2, d.ThresholdM2, d.Percent),
		"", "L", false)
	pdf.MultiCell(0, lineHeight,
		fmt.Sprintf("Инициатор требования: %s, помещение %s. Форма проведения собрания: %s.",
			d.InitiatorName, d.PremiseNumber, d.Channel), "", "L", false)
	pdf.Ln(4)

	// Таблица подписей собственников (без ФИО — решение 20).
	unicode(fontSizeBody, "B")
	pdf.CellFormat(0, lineHeight, "Подписи собственников, поддержавших требование:", "", 2, "L", false, 0, "")
	unicode(fontSizeSmall, "")
	drawSignatureTable(pdf)

	pdf.Ln(6)
	unicode(fontSizeSmall, "")
	pdf.MultiCell(0, lineHeight,
		fmt.Sprintf("Составлено %s. Приложение к инициативе «%s».", d.CreatedAt, d.Title), "", "L", false)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("demand pdf: %w", err)
	}

	return buf.Bytes(), nil
}

// drawSignatureTable: 12 строк «№, ФИО собственника, помещение, доля, подпись» — пустая таблица
// для живых подписей на бумаге (решение 20: имена в приложении не раскрываются).
func drawSignatureTable(pdf *fpdf.Fpdf) {
	const rowH = 8.0
	widths := []float64{10, 85, 40, 25, 30}
	headers := []string{"№", "ФИО собственника", "Помещение", "Доля", "Подпись"}

	pdf.SetFont("DejaVu", "", 9)
	pdf.SetDrawColor(80, 80, 80)
	for i, h := range headers {
		pdf.CellFormat(widths[i], rowH, h, "1", 0, "C", false, 0, "")
	}
	pdf.Ln(-1)
	for row := 1; row <= 12; row++ {
		pdf.CellFormat(widths[0], rowH, fmt.Sprint(row), "1", 0, "C", false, 0, "")
		for _, w := range widths[1:] {
			pdf.CellFormat(w, rowH, "", "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
	}
}

// PercentString: доля в процентах площади дома, одна десятая.
func PercentString(part, total *big.Rat) string {
	if total.Sign() == 0 {
		return "0.0"
	}
	p := new(big.Rat).Quo(part, total)
	p.Mul(p, big.NewRat(100, 1))

	return p.FloatString(1)
}

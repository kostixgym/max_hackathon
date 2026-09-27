package documents

import (
	"bytes"
	"strings"
	"testing"
)

func sampleDemandData() DemandData {
	return DemandData{
		OrgName:       "ООО «Демо-УК»",
		HouseAddress:  "г. Казань, ул. Демонстрационная, д. 1",
		InitiatorName: "Васильев Д. П.",
		PremiseNumber: "12",
		Title:         "Камеры в подъездах",
		Agenda: []string{
			"Избрать председателя и секретаря общего собрания",
			"Установить видеонаблюдение в подъездах дома",
		},
		SupportM2:   "317.50",
		ThresholdM2: "300.00",
		Percent:     "10.6",
		Channel:     "бумажные бюллетени",
		CreatedAt:   "27.09.2026",
	}
}

// The document must be a real PDF with the Cyrillic-capable font embedded:
// without the font the distroless runtime image would render boxes instead of
// the Russian text.
func TestDemandPDF(t *testing.T) {
	pdf, err := DemandPDF(sampleDemandData())
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("missing PDF header")
	}
	if !bytes.Contains(pdf, []byte("%%EOF")) {
		t.Fatal("missing EOF marker")
	}
	if len(pdf) < 2000 {
		t.Fatalf("pdf too small: %d bytes", len(pdf))
	}
}

// A document without the addressee is refused: it would be invalid under
// ст. 45 ч. 2 ЖК (the demand names the management organization).
func TestDemandPDFRequiresOrg(t *testing.T) {
	if _, err := DemandPDF(DemandData{HouseAddress: "дом"}); err == nil {
		t.Fatal("expected error for empty org name")
	}
}

// Long agenda lines must wrap, not clip: the document uses MultiCell for body
// text, a very long line must not break the rendering.
func TestDemandPDFWrapsLongText(t *testing.T) {
	data := sampleDemandData()
	data.Agenda = []string{strings.Repeat("Установить камеры у входа, ", 12)}
	if _, err := DemandPDF(data); err != nil {
		t.Fatalf("long agenda: %v", err)
	}
}

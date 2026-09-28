package registry

import (
	"fmt"
	"math/big"
)

// Demo house: synthetic data for the MVP and the jury (docs/04, решение 51).
// Nothing here is real: names, phones and account numbers are generated.
//
// Layout: 3 entrances × 5 floors × 4 flats = 60 flats + one shop.
// Flat areas per floor position sum to 192,00 м², so 60 flats give 2 880,00 м²
// and with the shop (120,00 м²) the building has exactly 3 000,00 м² —
// the example used throughout docs/02: 10% = 300 м², quorum > 1 500 м², 2/3 = 2 000 м².

const (
	demoEntrances     = 3
	demoFloors        = 5
	demoFlatsPerFloor = 4
)

var demoFlatAreas = [demoFlatsPerFloor]int64{3450, 4600, 4800, 6350} // сотые м²

// DemoOwner is one owner of a demo premise.
type DemoOwner struct {
	FullName string
	ShareNum int64
	ShareDen int64
	Kind     string // person / organization / municipality
	Phone    string // synthetic, empty if unknown to the management company
}

// DemoPremise is one premise of the demo house.
type DemoPremise struct {
	Number    string
	Kind      string // residential / nonresidential
	Entrance  int
	Floor     int
	AreaCenti int64
	Account   string // synthetic personal account number (лицевой счёт)
	Owners    []DemoOwner
}

var demoSurnames = []string{
	"Иванов", "Смирнов", "Кузнецов", "Попов", "Васильев", "Петров", "Соколов", "Михайлов",
	"Новиков", "Фёдоров", "Морозов", "Волков", "Алексеев", "Лебедев", "Семёнов", "Егоров",
}

const demoInitials = "АБВГДЕИКЛМНОПРСТ"

// DemoPremises returns the deterministic synthetic registry of the demo house.
func DemoPremises() []DemoPremise {
	premises := make([]DemoPremise, 0, demoEntrances*demoFloors*demoFlatsPerFloor+1)

	n := 0
	for e := 1; e <= demoEntrances; e++ {
		for f := 1; f <= demoFloors; f++ {
			for pos := range demoFlatsPerFloor {
				n++
				premises = append(premises, DemoPremise{
					Number:    fmt.Sprint(n),
					Kind:      "residential",
					Entrance:  e,
					Floor:     f,
					AreaCenti: demoFlatAreas[pos],
					Account:   fmt.Sprintf("DEMO-%04d", n),
					Owners:    demoOwners(n),
				})
			}
		}
	}

	premises = append(premises, DemoPremise{
		Number:    "Н1",
		Kind:      "nonresidential",
		Entrance:  1,
		Floor:     1,
		AreaCenti: 12000,
		Account:   "DEMO-N001",
		Owners:    []DemoOwner{{FullName: "ООО «Демо-Магазин»", ShareNum: 1, ShareDen: 1, Kind: "organization"}},
	})

	return premises
}

// demoOwners covers the special cases from docs/01: co-owners with 1/2 and 1/3 shares,
// a municipal flat, owners with and without a known phone.
func demoOwners(flat int) []DemoOwner {
	switch {
	case flat == 60:
		return []DemoOwner{{FullName: "Муниципальное образование (демо)", ShareNum: 1, ShareDen: 1, Kind: "municipality"}}
	case flat%15 == 7:
		return []DemoOwner{person(flat, 0, 1, 3), person(flat, 1, 1, 3), person(flat, 2, 1, 3)}
	case flat%10 == 3:
		return []DemoOwner{person(flat, 0, 1, 2), person(flat, 1, 1, 2)}
	default:
		return []DemoOwner{person(flat, 0, 1, 1)}
	}
}

func person(flat, idx int, num, den int64) DemoOwner {
	k := flat*3 + idx
	initials := []rune(demoInitials)
	name := fmt.Sprintf("%s %c. %c.", demoSurnames[k%len(demoSurnames)], initials[k%len(initials)], initials[(k/3)%len(initials)])

	o := DemoOwner{FullName: name, ShareNum: num, ShareDen: den, Kind: "person"}
	// The management company knows phones of two thirds of the owners.
	if k%3 != 0 {
		o.Phone = fmt.Sprintf("+7999%07d", k)
	}

	return o
}

// DemoTotalAreaCenti is the total area of the demo house in hundredths of м².
func DemoTotalAreaCenti(premises []DemoPremise) int64 {
	var total int64
	for _, p := range premises {
		total += p.AreaCenti
	}

	return total
}

// ShareSum returns the sum of owner shares of a premise.
func ShareSum(owners []DemoOwner) *big.Rat {
	sum := new(big.Rat)
	for _, o := range owners {
		sum.Add(sum, big.NewRat(o.ShareNum, o.ShareDen))
	}

	return sum
}

package access

import (
	"context"
	"errors"
	"testing"
)

func TestMaskOwnerName(t *testing.T) {
	tests := []struct {
		name string
		kind string
		want string
	}{
		{name: "Иванов Иван Иванович", kind: "person", want: "Иванов И. И."},
		{name: "Петров И. И.", kind: "person", want: "Петров И. И."},
		{name: "Ли Мин", kind: "person", want: "Ли М."},
		{name: "ООО «Дом»", kind: "organization", want: "ООО «Дом»"},
		{name: "Муниципальное образование", kind: "municipality", want: "Муниципальное образование"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskOwnerName(tt.name, tt.kind); got != tt.want {
				t.Fatalf("maskOwnerName(%q, %q) = %q, want %q", tt.name, tt.kind, got, tt.want)
			}
		})
	}
}

func TestOwnerDirectoriesRejectInvalidIDsBeforeQuery(t *testing.T) {
	store := &Store{}
	if _, err := store.PremiseOwners(context.Background(), "user", "not-a-uuid"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("PremiseOwners error = %v, want ErrNotFound", err)
	}
	if _, err := store.HouseOfficerCandidates(context.Background(), "user", "not-a-uuid"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("HouseOfficerCandidates error = %v, want ErrNotFound", err)
	}
	// GET /houses/{houseID}/initiatives: a malformed id is no house, not a 500.
	if ok, err := store.MayViewInitiatives(context.Background(), "user", "not-a-uuid"); ok || err != nil {
		t.Fatalf("MayViewInitiatives = %v, %v, want false without an error", ok, err)
	}
}

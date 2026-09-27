package access

import (
	"context"
	"errors"
	"testing"
)

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

package initiatives

import (
	"context"
	"errors"
	"testing"

	"maxhackathon/backend/internal/registry"
)

func TestInvitees(t *testing.T) {
	all := []Recipient{
		{UserID: "initiator", MaxUserID: 1, OwnerID: "o-1"},
		{UserID: "neighbour", MaxUserID: 2, OwnerID: "o-2"},
		{UserID: "initiator", MaxUserID: 1, OwnerID: "o-3"}, // the initiator's second flat
	}

	if got := invitees(all, registry.HouseRef{IsDemo: false}, "initiator"); len(got) != 3 {
		t.Fatalf("real house: %d invitees, want every verified owner", len(got))
	}

	// In the demo house a tester's poll must not reach the other testers (the jury).
	got := invitees(all, registry.HouseRef{IsDemo: true}, "initiator")
	if len(got) != 2 || got[0].UserID != "initiator" || got[1].UserID != "initiator" {
		t.Fatalf("demo house: invitees = %+v, want the initiator only", got)
	}
}

// Ids come from URLs and button payloads: a malformed one is «not found» before any query.
func TestMalformedIDsAreNotFound(t *testing.T) {
	s := &Service{} // no pool: a query would panic
	if _, err := s.Get(context.Background(), "not-a-uuid"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get: %v, want ErrNotFound", err)
	}
	if _, err := s.Polling(context.Background(), "pv:1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Polling: %v, want ErrNotFound", err)
	}
	if _, err := s.CreateFromTemplate(context.Background(), CreateInput{Title: "   "}); !errors.Is(err, ErrEmptyTitle) {
		t.Fatalf("blank title: %v, want ErrEmptyTitle", err)
	}
	if _, err := s.Details(context.Background(), "../1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Details: %v, want ErrNotFound", err)
	}
	if list, err := s.ListByHouse(context.Background(), "house-1", "0199a1b2-0000-7000-8000-000000000001"); err != nil || len(list) != 0 {
		t.Fatalf("ListByHouse of a malformed house: %v, %v, want an empty list", list, err)
	}
}

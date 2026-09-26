package bot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/registry"
)

type fakePolling struct {
	stage string
	err   error
}

func (f fakePolling) Polling(_ context.Context, id string) (initiatives.Polling, error) {
	if f.err != nil {
		return initiatives.Polling{}, f.err
	}

	// 15:00 UTC is 18:00 in Moscow, the default zone of a house.
	endsAt := time.Date(2026, time.October, 3, 15, 0, 0, 0, time.UTC)

	return initiatives.Polling{ID: id, HouseID: "house-1", Title: "Камеры", Stage: f.stage, PollEndsAt: &endsAt}, nil
}

type fakeHouseReader struct{}

func (fakeHouseReader) House(_ context.Context, id string) (registry.HouseRef, error) {
	return registry.HouseRef{ID: id, InviteSlug: "demo-slug_1", Address: "г. Казань, ул. Демонстрационная, д. 1", IsDemo: true}, nil
}

func inviteJob(t *testing.T) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"initiative_id": testInitiative, "max_user_id": 42})
	if err != nil {
		t.Fatal(err)
	}

	return payload
}

func TestPollInviteSendsVotingButtons(t *testing.T) {
	msgs := &fakeMessages{}
	inviter := &PollInviter{Messages: msgs, Initiatives: fakePolling{stage: "poll"}, Houses: fakeHouseReader{},
		Me: Identity{UserID: 555, Username: "dom_test_bot"}}

	if err := inviter.HandleJob(context.Background(), inviteJob(t)); err != nil {
		t.Fatal(err)
	}
	if len(msgs.sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(msgs.sent))
	}
	body := msgs.sent[0].body
	buttons := body.Attachments[0].Payload.Buttons
	if len(buttons) != 3 || buttons[0][0].Payload != "pv:"+testInitiative+":for" ||
		buttons[0][1].Payload != "pv:"+testInitiative+":against" || buttons[1][0].Payload != "pv:"+testInitiative+":question" {
		t.Fatalf("buttons = %+v", buttons)
	}
	// The app button opens the house of the poll.
	if app := buttons[2][0]; app.Type != model.ButtonOpenApp || app.Payload != "demo-slug_1" || app.WebApp != "dom_test_bot" {
		t.Fatalf("app button = %+v", app)
	}
	if !strings.Contains(body.Text, "Опрос идёт до 3 октября, 18:00") {
		t.Fatalf("the deadline in the local time of the house is missing: %q", body.Text)
	}
}

// A job may run long after it was queued: an initiative that is hidden, cancelled
// or past its poll gets no voting buttons, and the job is not retried.
func TestPollInviteSkipsClosedPolls(t *testing.T) {
	cases := map[string]fakePolling{
		"hidden or deleted":   {err: initiatives.ErrNotFound},
		"cancelled":           {stage: "canceled"},
		"moved to a meeting":  {stage: "meeting"},
		"still a draft (bug)": {stage: "draft"},
	}
	for name, polling := range cases {
		t.Run(name, func(t *testing.T) {
			msgs := &fakeMessages{}
			inviter := &PollInviter{Messages: msgs, Initiatives: polling, Houses: fakeHouseReader{}}
			if err := inviter.HandleJob(context.Background(), inviteJob(t)); err != nil {
				t.Fatalf("err = %v, want nil (nothing to retry)", err)
			}
			if len(msgs.sent) != 0 {
				t.Fatalf("sent %d messages, want 0", len(msgs.sent))
			}
		})
	}
}

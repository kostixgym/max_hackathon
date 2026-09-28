package bot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/poll"
	"maxhackathon/backend/internal/registry"
)

type fakePolling struct {
	stage      string
	err        error
	endsBefore bool   // the term is before testNow
	initiator  string // the user id of the initiator
}

func (f fakePolling) Polling(_ context.Context, id string) (initiatives.Polling, error) {
	if f.err != nil {
		return initiatives.Polling{}, f.err
	}

	// 15:00 UTC is 18:00 in Moscow, the default zone of a house.
	endsAt := time.Date(2026, time.October, 3, 15, 0, 0, 0, time.UTC)
	if f.endsBefore {
		endsAt = testNow.Add(-time.Minute)
	}

	p := initiatives.Polling{ID: id, HouseID: "house-1", Title: "Камеры", Stage: f.stage, PollEndsAt: &endsAt}
	if f.initiator != "" {
		p.InitiatorUserID = &f.initiator
	}

	return p, nil
}

// fakeProgress: a demo house of 3 000 м² with forCenti hundredths of м² «за».
type fakeProgress struct{ forCenti int64 }

func (f fakeProgress) Progress(_ context.Context, id string) (poll.Progress, error) {
	return poll.Progress{InitiativeID: id, Stage: "poll", TotalCenti: 300000, ForNum: f.forCenti, ForDen: 1,
		AgainstNum: 4800, AgainstDen: 1, VotesFor: 3, VotesAgainst: 1}, nil
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
		Progress: fakeProgress{forCenti: 12400}, Me: Identity{UserID: 555, Username: "dom_test_bot"},
		Now: func() time.Time { return testNow }}

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
	if body.Format != model.FormatHTML || !strings.Contains(body.Text, "Опрос идёт до <b>3 октября, 18:00</b>") {
		t.Fatalf("the deadline in the local time of the house is missing: %q", body.Text)
	}
	// The support so far against the threshold of a demand to the company.
	if !strings.Contains(body.Text, "Поддержали: <b>124,00 м²</b> из 300,00 м²") {
		t.Fatalf("the support line is missing: %q", body.Text)
	}
}

type fakePollVoteReader struct {
	voted bool
	err   error
}

func (f fakePollVoteReader) MyVote(context.Context, string, string) (poll.MyVote, bool, error) {
	return poll.MyVote{}, f.voted, f.err
}

func TestPollReminderSkipsOwnersWhoVoted(t *testing.T) {
	msgs := &fakeMessages{}
	inviter := &PollInviter{Messages: msgs, Initiatives: fakePolling{stage: "poll"}, Houses: fakeHouseReader{}, Me: Identity{UserID: 555, Username: "dom_test_bot"}, Now: func() time.Time { return testNow }}
	reminder := &Reminder{Inviter: inviter, Votes: fakePollVoteReader{voted: true}}
	payload, _ := json.Marshal(map[string]any{"initiative_id": testInitiative, "user_id": "user-1", "max_user_id": 42})
	if err := reminder.HandleJob(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if len(msgs.sent) != 0 {
		t.Fatalf("sent %d reminders to an owner who voted", len(msgs.sent))
	}
	reminder.Votes = fakePollVoteReader{}
	if err := reminder.HandleJob(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if len(msgs.sent) != 1 {
		t.Fatalf("sent %d reminders to an owner who has not voted", len(msgs.sent))
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
		// The invitation waited for the morning longer than the poll lasted.
		"term is over": {stage: "poll", endsBefore: true},
	}
	for name, polling := range cases {
		t.Run(name, func(t *testing.T) {
			msgs := &fakeMessages{}
			inviter := &PollInviter{Messages: msgs, Initiatives: polling, Houses: fakeHouseReader{},
				Now: func() time.Time { return testNow }}
			if err := inviter.HandleJob(context.Background(), inviteJob(t)); err != nil {
				t.Fatalf("err = %v, want nil (nothing to retry)", err)
			}
			if len(msgs.sent) != 0 {
				t.Fatalf("sent %d messages, want 0", len(msgs.sent))
			}
		})
	}
}

// The initiator answers questions instead of asking them: their message has no
// «Есть вопрос» but the link to call the neighbours (решение 78).
func TestPollInviteToInitiator(t *testing.T) {
	msgs := &fakeMessages{}
	inviter := &PollInviter{Messages: msgs, Initiatives: fakePolling{stage: "poll"}, Houses: fakeHouseReader{},
		Me: Identity{UserID: 555, Username: "dom_test_bot"}, Now: func() time.Time { return testNow }}
	payload, err := json.Marshal(map[string]any{"initiative_id": testInitiative, "max_user_id": 42, "initiator": true})
	if err != nil {
		t.Fatal(err)
	}

	if err := inviter.HandleJob(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	body := msgs.sent[0].body
	link := "https://max.ru/dom_test_bot?start=poll_" + testInitiative
	if !strings.Contains(body.Text, "Вы инициатор") || !strings.Contains(body.Text, `<a href="`+link+`">`) {
		t.Fatalf("text = %s", body.Text)
	}
	buttons := body.Attachments[0].Payload.Buttons
	if len(buttons) != 3 || buttons[1][0].Type != model.ButtonClipboard || buttons[1][0].Payload != link {
		t.Fatalf("buttons = %+v, want the votes, the link to copy and the app", buttons)
	}
	for _, row := range buttons {
		for _, btn := range row {
			if strings.HasSuffix(btn.Payload, ":question") {
				t.Fatalf("the initiator got «Есть вопрос»: %+v", btn)
			}
		}
	}
}

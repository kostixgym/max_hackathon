package initiatives

import (
	"slices"
	"testing"
	"time"
)

func TestActions(t *testing.T) {
	initiator := "user-initiator"
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	later, earlier := now.Add(time.Hour), now.Add(-time.Second)
	in := func(stage string) Initiative {
		return Initiative{Stage: stage, InitiatorUserID: &initiator, PollEndsAt: &later}
	}
	ended := in(StagePoll)
	ended.PollEndsAt = &earlier
	endsNow := in(StagePoll)
	endsNow.PollEndsAt = &now
	lead := Viewer{UserID: initiator, Owner: true}
	neighbour := Viewer{UserID: "user-neighbour", Owner: true}
	resident := Viewer{UserID: "user-resident", Owner: false}

	cases := []struct {
		name       string
		initiative Initiative
		viewer     Viewer
		startPoll  string // "" — allowed
		castVote   string
	}{
		{"initiator, draft", in(StageDraft), lead, "", ReasonWrongStage},
		{"initiator, poll", in(StagePoll), lead, ReasonPollAlreadyStarted, ""},
		{"initiator, meeting", in(StageMeeting), lead, ReasonPollAlreadyStarted, ReasonWrongStage},
		{"initiator, canceled", in(StageCanceled), lead, ReasonWrongStage, ReasonWrongStage},
		{"neighbour owner, poll", in(StagePoll), neighbour, ReasonNotInitiator, ""},
		{"resident, poll", in(StagePoll), resident, ReasonNotInitiator, ReasonOwnerVerificationRequired},
		{"resident, demand", in(StageDemand), resident, ReasonNotInitiator, ReasonWrongStage},
		// Решение 77: the poll takes votes until its term, the term itself is already closed.
		{"owner, term is over", ended, neighbour, ReasonNotInitiator, ReasonPollFinished},
		{"owner, term is now", endsNow, neighbour, ReasonNotInitiator, ReasonPollFinished},
		{"resident, term is over", ended, resident, ReasonNotInitiator, ReasonPollFinished},
		// An initiative whose initiator deleted the account (решение 28) has no initiator.
		{"no initiator", Initiative{Stage: StageDraft}, lead, ReasonNotInitiator, ReasonWrongStage},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actions := c.initiative.Actions(c.viewer, now)

			codes := make([]string, 0, len(actions))
			for _, a := range actions {
				codes = append(codes, a.Code)
				if a.Allowed != (a.Reason == "") {
					t.Errorf("%s: allowed=%v with reason %q", a.Code, a.Allowed, a.Reason)
				}
			}
			// The card always lists every action the mini-app knows, in a fixed order.
			want := []string{ActionEdit, ActionStartPoll, ActionCastPollVote, ActionSelectPathA,
				ActionSelectPathB, ActionCreateMeeting, ActionCancel}
			if !slices.Equal(codes, want) {
				t.Fatalf("codes = %v, want %v", codes, want)
			}

			for _, a := range actions {
				switch a.Code {
				case ActionStartPoll:
					if a.Reason != c.startPoll {
						t.Errorf("start_poll: reason %q, want %q", a.Reason, c.startPoll)
					}
				case ActionCastPollVote:
					if a.Reason != c.castVote {
						t.Errorf("cast_poll_vote: reason %q, want %q", a.Reason, c.castVote)
					}
				case ActionSelectPathA:
					if a.Reason != c.initiative.pathChoiceBlocked(c.viewer, false) {
						t.Errorf("select_path_a: reason %q", a.Reason)
					}
				case ActionSelectPathB:
					if a.Reason != c.initiative.pathChoiceBlocked(c.viewer, true) {
						t.Errorf("select_path_b: reason %q", a.Reason)
					}
				default:
					// Steps 1.4–1.6 are not done: the button must not work yet.
					if a.Allowed || a.Reason != ReasonNotImplemented {
						t.Errorf("%s: %+v, want not_implemented", a.Code, a)
					}
				}
			}
		})
	}
}

// One rule of an open poll for the card, the poll module and the bot.
func TestPollingOpen(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	later, earlier := now.Add(time.Minute), now.Add(-time.Minute)
	cases := []struct {
		p    Polling
		open bool
	}{
		{Polling{Stage: StagePoll, PollEndsAt: &later}, true},
		{Polling{Stage: StagePoll}, true}, // no term: open until the path is chosen
		{Polling{Stage: StagePoll, PollEndsAt: &now}, false},
		{Polling{Stage: StagePoll, PollEndsAt: &earlier}, false},
		{Polling{Stage: StageDraft, PollEndsAt: &later}, false},
		{Polling{Stage: StageDemand, PollEndsAt: &later}, false},
	}
	for _, c := range cases {
		if got := c.p.Open(now); got != c.open {
			t.Errorf("%+v: open = %v, want %v", c.p, got, c.open)
		}
	}
}

func TestIsLedBy(t *testing.T) {
	initiator, author := "u-1", "u-2"
	in := Initiative{InitiatorUserID: &initiator, AuthorUserID: &author}
	if !in.IsLedBy(initiator) || !in.IsLedBy(author) || in.IsLedBy("u-3") || (Initiative{}).IsLedBy("") {
		t.Fatal("IsLedBy is true exactly for the initiator and the author")
	}
}

package initiatives

import (
	"slices"
	"testing"
)

func TestActions(t *testing.T) {
	initiator := "user-initiator"
	in := func(stage string) Initiative {
		return Initiative{Stage: stage, InitiatorUserID: &initiator}
	}
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
		// An initiative whose initiator deleted the account (решение 28) has no initiator.
		{"no initiator", Initiative{Stage: StageDraft}, lead, ReasonNotInitiator, ReasonWrongStage},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actions := c.initiative.Actions(c.viewer)

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

func TestIsLedBy(t *testing.T) {
	initiator, author := "u-1", "u-2"
	in := Initiative{InitiatorUserID: &initiator, AuthorUserID: &author}
	if !in.IsLedBy(initiator) || !in.IsLedBy(author) || in.IsLedBy("u-3") || (Initiative{}).IsLedBy("") {
		t.Fatal("IsLedBy is true exactly for the initiator and the author")
	}
}

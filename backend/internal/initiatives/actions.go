package initiatives

import "time"

// Actions of the initiative card (docs/API_DESCRIPTION.md, GET /initiatives/{id}): the
// mini-app enables a button only by the verdict of the server, and the server checks
// the same rule again when the action is called.

// Action codes, in the order of the card.
const (
	ActionEdit          = "edit"
	ActionStartPoll     = "start_poll"
	ActionCastPollVote  = "cast_poll_vote"
	ActionSelectPathA   = "select_path_a"
	ActionSelectPathB   = "select_path_b"
	ActionCreateMeeting = "create_meeting"
	ActionCancel        = "cancel"
)

// Reason codes: why an action is not allowed now.
const (
	ReasonOwnerVerificationRequired = "owner_verification_required"
	ReasonNotInitiator              = "not_initiator"
	ReasonWrongStage                = "wrong_stage"
	ReasonPollAlreadyStarted        = "poll_already_started"
	// ReasonPollFinished: the term of the poll is over, the votes are no longer taken
	// (решение 77). The initiative stays on the poll stage until the path is chosen.
	ReasonPollFinished = "poll_finished"
	// ReasonNotImplemented: the action belongs to a step of the plan that is not done
	// yet (edit, paths A and B, meeting, cancel — steps 1.4–1.6). The step replaces it
	// with the rule of the action, and the button of the mini-app turns on by itself.
	ReasonNotImplemented = "not_implemented"
)

// Action is one button of the card.
type Action struct {
	Code    string
	Allowed bool
	Reason  string // empty when allowed
}

// Viewer is the user who opens the card.
type Viewer struct {
	UserID string
	// Owner: a verified owner of a premise of the house. Only owners vote (docs/01).
	Owner bool
}

// Actions returns every action of the card with its verdict for the viewer at the moment now.
func (in Initiative) Actions(v Viewer, now time.Time) []Action {
	return []Action{
		{Code: ActionEdit, Reason: ReasonNotImplemented},
		verdict(ActionStartPoll, in.startPollBlocked(v.UserID)),
		verdict(ActionCastPollVote, in.votingBlocked(v, now)),
		{Code: ActionSelectPathA, Reason: ReasonNotImplemented},
		{Code: ActionSelectPathB, Reason: ReasonNotImplemented},
		{Code: ActionCreateMeeting, Reason: ReasonNotImplemented},
		{Code: ActionCancel, Reason: ReasonNotImplemented},
	}
}

func verdict(code, reason string) Action {
	return Action{Code: code, Allowed: reason == "", Reason: reason}
}

// startPollBlocked: the initiator starts the poll of a draft (docs/02, шаг 2).
// StartPoll checks the same rule.
func (in Initiative) startPollBlocked(userID string) string {
	switch {
	case in.InitiatorUserID == nil || *in.InitiatorUserID != userID:
		return ReasonNotInitiator
	case in.Stage == StageDraft:
		return ""
	case in.Stage == StageCanceled:
		return ReasonWrongStage
	default:
		return ReasonPollAlreadyStarted
	}
}

// votingBlocked: verified owners vote while the poll is open (решения 3, 77). The
// poll module checks the same rule when the vote comes.
func (in Initiative) votingBlocked(v Viewer, now time.Time) string {
	switch {
	case in.Stage != StagePoll:
		return ReasonWrongStage
	case !pollOpen(in.Stage, in.PollEndsAt, now):
		return ReasonPollFinished
	case !v.Owner:
		return ReasonOwnerVerificationRequired
	default:
		return ""
	}
}

// Open reports whether the poll takes votes at the moment now: the initiative is on
// the poll stage and the term has not ended (решения 3, 77). The poll module, the bot
// and the card of the mini-app use this one rule.
func (p Polling) Open(now time.Time) bool {
	return pollOpen(p.Stage, p.PollEndsAt, now)
}

// TakesQuestions reports whether the neighbours may ask the initiator a question now:
// while the initiative goes on (решение 78).
func (p Polling) TakesQuestions() bool {
	return takesQuestions(p.Stage)
}

func pollOpen(stage string, endsAt *time.Time, now time.Time) bool {
	return stage == StagePoll && (endsAt == nil || now.Before(*endsAt))
}

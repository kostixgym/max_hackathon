package initiatives

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

// Actions returns every action of the card with its verdict for the viewer.
func (in Initiative) Actions(v Viewer) []Action {
	return []Action{
		{Code: ActionEdit, Reason: ReasonNotImplemented},
		verdict(ActionStartPoll, in.startPollBlocked(v.UserID)),
		verdict(ActionCastPollVote, in.votingBlocked(v)),
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

// votingBlocked: verified owners vote while the poll is on (решение 3). The poll
// module checks the same rule when the vote comes.
func (in Initiative) votingBlocked(v Viewer) string {
	switch {
	case in.Stage != StagePoll:
		return ReasonWrongStage
	case !v.Owner:
		return ReasonOwnerVerificationRequired
	default:
		return ""
	}
}

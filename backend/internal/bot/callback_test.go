package bot

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/poll"
)

const testInitiative = "22222222-2222-7222-8222-222222222222"

type fakeUsers struct {
	mu  sync.Mutex
	ids []int64
}

func (f *fakeUsers) EnsureUser(_ context.Context, maxID int64) (access.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ids = append(f.ids, maxID)

	return access.User{ID: "user-42", MaxUserID: maxID}, nil
}

type fakeVotes struct {
	mu   sync.Mutex
	cast []poll.CastInput
	err  error
}

func (f *fakeVotes) CastVote(_ context.Context, in poll.CastInput) (poll.CastResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return poll.CastResult{}, f.err
	}
	f.cast = append(f.cast, in)

	return poll.CastResult{Choice: in.Choice, PremiseNumber: "45", WeightNum: 5230, WeightDen: 2}, nil
}

type fakeAnswers struct {
	mu       sync.Mutex
	calls    []string // callback_id
	texts    []string
	messages []*model.NewMessageBody // the updated poll message, when the answer carries one
}

func (f *fakeAnswers) AnswerOnCallback(_ context.Context, callbackID string, answer model.CallbackAnswer) (model.SimpleQueryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, callbackID)
	if answer.Notification != nil {
		f.texts = append(f.texts, *answer.Notification)
	}
	f.messages = append(f.messages, answer.Message)

	return model.SimpleQueryResult{}, nil
}

func callbackUpdate(callbackID, payload string, maxUserID int64) model.Update {
	return model.Update{
		UpdateType: model.UpdateMessageCallback,
		Callback: &model.Callback{
			CallbackID: callbackID,
			Payload:    payload,
			User:       model.User{UserID: maxUserID, FirstName: "Анна"},
		},
	}
}

// testNow is before the term of fakePolling (3 October, 18:00 in Moscow).
var testNow = time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)

func newVoteBot(votes *fakeVotes) *Bot {
	return &Bot{
		Messages: &fakeMessages{},
		Houses:   nil,
		Me:       Identity{UserID: 555, Username: "dom_test_bot"},
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Users:    &fakeUsers{},
		Votes:    votes,
		Answers:  &fakeAnswers{},
		Now:      func() time.Time { return testNow },
	}
}

func TestCallbackVote(t *testing.T) {
	for _, tc := range []struct {
		action string
		choice string
	}{
		{"for", poll.ChoiceFor},
		{"against", poll.ChoiceAgainst},
	} {
		votes := &fakeVotes{}
		b := newVoteBot(votes)
		b.Handle(context.Background(), callbackUpdate("cb-1", "pv:"+testInitiative+":"+tc.action, 42))

		if len(votes.cast) != 1 {
			t.Fatalf("action %s: votes = %d, want 1", tc.action, len(votes.cast))
		}
		if votes.cast[0].InitiativeID != testInitiative || votes.cast[0].Choice != tc.choice {
			t.Fatalf("action %s: vote = %+v", tc.action, votes.cast[0])
		}
	}
}

func TestCallbackQuestionDoesNotVote(t *testing.T) {
	votes := &fakeVotes{}
	b := newVoteBot(votes)
	answers := &fakeAnswers{}
	b.Answers = answers

	b.Handle(context.Background(), callbackUpdate("cb-q", "pv:"+testInitiative+":question", 42))

	if len(votes.cast) != 0 {
		t.Fatalf("question pressed: votes = %+v, want none", votes.cast)
	}
	// Without the questions module the press is still answered, and visibly.
	if len(answers.texts) != 1 || len(b.Messages.(*fakeMessages).sent) != 1 {
		t.Fatalf("answers = %q, messages = %d", answers.texts, len(b.Messages.(*fakeMessages).sent))
	}
}

func TestCallbackVoteErrors(t *testing.T) {
	cases := map[string]struct {
		voteErr error
		want    string
	}{
		"not owner":   {poll.ErrNotOwner, "подтверждённые собственники"},
		"poll closed": {poll.ErrPollClosed, "Опрос завершён"},
		"no weight":   {poll.ErrNoWeight, "реестра"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			votes := &fakeVotes{err: tc.voteErr}
			b := newVoteBot(votes)
			answers := &fakeAnswers{}
			b.Answers = answers

			b.Handle(context.Background(), callbackUpdate("cb-e", "pv:"+testInitiative+":for", 42))

			if len(answers.texts) != 1 || !strings.Contains(answers.texts[0], tc.want) {
				t.Fatalf("answer = %q, want it to contain %q", answers.texts, tc.want)
			}
		})
	}
}

// MAX does not show the notification of a callback answer (проверка 0.6): a refused
// vote must also come as a message, or the press looks dead.
func TestCallbackVoteErrorIsVisible(t *testing.T) {
	b := newVoteBot(&fakeVotes{err: poll.ErrNotOwner})

	b.Handle(context.Background(), callbackUpdate("cb-e", "pv:"+testInitiative+":for", 42))

	sent := b.Messages.(*fakeMessages).sent
	if len(sent) != 1 || !strings.Contains(sent[0].body.Text, "подтверждённые собственники") {
		t.Fatalf("messages = %+v, want the refusal as a message", sent)
	}
}

func TestCallbackIgnoresForeignPayloads(t *testing.T) {
	votes := &fakeVotes{}
	b := newVoteBot(votes)

	for _, payload := range []string{"", "other:1:for", "pv:", "pv::for", "pv:" + testInitiative + ":maybe", "pv:" + testInitiative} {
		b.Handle(context.Background(), callbackUpdate("cb-x", payload, 42))
	}

	if len(votes.cast) != 0 {
		t.Fatalf("foreign payloads voted: %+v", votes.cast)
	}
}

// A callback without the presser must not create a user with MAX id 0.
func TestCallbackWithoutUserIsIgnored(t *testing.T) {
	votes := &fakeVotes{}
	b := newVoteBot(votes)
	users := &fakeUsers{}
	b.Users = users

	b.Handle(context.Background(), callbackUpdate("cb-0", "pv:"+testInitiative+":for", 0))

	if len(users.ids) != 0 || len(votes.cast) != 0 {
		t.Fatalf("users = %v, votes = %+v, want none", users.ids, votes.cast)
	}
}

func TestCallbackWithoutDepsIsIgnored(t *testing.T) {
	b := &Bot{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}                          // no Users/Votes/Answers
	b.Handle(context.Background(), callbackUpdate("cb-1", "pv:"+testInitiative+":for", 42)) // must not panic
}

// After a vote the poll message itself shows the current choice and keeps the
// buttons to change it (решение 68).
func TestCallbackUpdatesPollMessage(t *testing.T) {
	b := newVoteBot(&fakeVotes{})
	answers := &fakeAnswers{}
	b.Answers, b.Initiatives, b.HousesByID = answers, fakePolling{stage: "poll"}, fakeHouseReader{}

	b.Handle(context.Background(), callbackUpdate("cb-1", "pv:"+testInitiative+":for", 42))

	msg := answers.messages[0]
	if msg == nil || msg.Format != model.FormatHTML || !strings.Contains(msg.Text, "<b>Ваш голос: «за», 26,15 м² (кв. 45)</b>") {
		t.Fatalf("updated message = %+v, want the current choice in bold", msg)
	}
	buttons := msg.Attachments[0].Payload.Buttons
	if len(buttons) != 3 || buttons[0][0].Type != model.ButtonCallback {
		t.Fatalf("the voting buttons must stay while the poll is on: %+v", buttons)
	}
	// The chosen option is marked on its button.
	if buttons[0][0].Text != "✅ Поддерживаю" || buttons[0][1].Text != "Против" {
		t.Fatalf("vote buttons = %q, %q", buttons[0][0].Text, buttons[0][1].Text)
	}
}

// A press on a message of a closed poll removes its voting buttons.
func TestCallbackClosedPollRemovesButtons(t *testing.T) {
	b := newVoteBot(&fakeVotes{err: poll.ErrPollClosed})
	answers := &fakeAnswers{}
	b.Answers, b.Initiatives, b.HousesByID = answers, fakePolling{stage: "meeting"}, fakeHouseReader{}

	b.Handle(context.Background(), callbackUpdate("cb-1", "pv:"+testInitiative+":for", 42))

	msg := answers.messages[0]
	if msg == nil || !strings.Contains(msg.Text, "Опрос завершён") {
		t.Fatalf("updated message = %+v, want «Опрос завершён»", msg)
	}
	// The voting buttons are gone; a question can still be asked while the initiative goes on.
	buttons := msg.Attachments[0].Payload.Buttons
	if len(buttons) != 2 || buttons[0][0].Payload != "pv:"+testInitiative+":question" || buttons[1][0].Type != model.ButtonOpenApp {
		t.Fatalf("after the poll: %+v, want «Есть вопрос» and the app button", buttons)
	}
}

// The term closes the poll even before the initiator moves on (решение 77).
func TestCallbackAfterTermRemovesVoteButtons(t *testing.T) {
	b := newVoteBot(&fakeVotes{err: poll.ErrPollClosed})
	answers := &fakeAnswers{}
	b.Answers, b.Initiatives, b.HousesByID = answers, fakePolling{stage: "poll"}, fakeHouseReader{}
	b.Now = func() time.Time { return time.Date(2026, time.October, 3, 15, 0, 0, 0, time.UTC) } // the term itself

	b.Handle(context.Background(), callbackUpdate("cb-1", "pv:"+testInitiative+":for", 42))

	msg := answers.messages[0]
	if msg == nil || !strings.Contains(msg.Text, "Опрос завершён") || strings.Contains(msg.Text, "Опрос идёт до") {
		t.Fatalf("updated message = %+v, want the closed poll", msg)
	}
	if buttons := msg.Attachments[0].Payload.Buttons; len(buttons) != 2 {
		t.Fatalf("after the term: %+v, want no voting buttons", buttons)
	}
}

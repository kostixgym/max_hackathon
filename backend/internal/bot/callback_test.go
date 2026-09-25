package bot

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

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
	mu    sync.Mutex
	calls []string // callback_id
	texts []string
}

func (f *fakeAnswers) AnswerOnCallback(_ context.Context, callbackID string, answer model.CallbackAnswer) (model.SimpleQueryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, callbackID)
	if answer.Notification != nil {
		f.texts = append(f.texts, *answer.Notification)
	}

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

func newVoteBot(votes *fakeVotes) *Bot {
	return &Bot{
		Messages: nil,
		Houses:   nil,
		Me:       Identity{UserID: 555, Username: "dom_test_bot"},
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Users:    &fakeUsers{},
		Votes:    votes,
		Answers:  &fakeAnswers{},
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
	if len(answers.texts) != 1 {
		t.Fatalf("answers = %q", answers.texts)
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

func TestCallbackWithoutDepsIsIgnored(t *testing.T) {
	b := &Bot{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}                          // no Users/Votes/Answers
	b.Handle(context.Background(), callbackUpdate("cb-1", "pv:"+testInitiative+":for", 42)) // must not panic
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	maxapi "github.com/max-messenger/max-bot-api-client-go/v2"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/notify"
	"maxhackathon/backend/internal/poll"
	"maxhackathon/backend/internal/registry"
)

const e2eInitiativeID = "11111111-1111-7111-8111-111111111111"

type e2eHouses struct{}

func (e2eHouses) HouseBySlug(_ context.Context, slug string) (registry.HouseSummary, error) {
	if slug != "demo-slug_1" {
		return registry.HouseSummary{}, registry.ErrNotFound
	}

	return registry.HouseSummary{ID: "house-1", InviteSlug: slug, Address: "демо-адрес", IsDemo: true}, nil
}

func (e2eHouses) House(_ context.Context, id string) (registry.HouseRef, error) {
	return registry.HouseRef{ID: id, Address: "демо-адрес", IsDemo: true}, nil
}

type e2eUsers struct{}

func (e2eUsers) EnsureUser(_ context.Context, maxUserID int64) (access.User, error) {
	return access.User{ID: fmt.Sprintf("user-%d", maxUserID), MaxUserID: maxUserID}, nil
}

type e2eVotes struct {
	mu   sync.Mutex
	cast []poll.CastInput
}

func (v *e2eVotes) CastVote(_ context.Context, in poll.CastInput) (poll.CastResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.cast = append(v.cast, in)

	return poll.CastResult{Choice: in.Choice, PremiseNumber: "45", WeightNum: 2615, WeightDen: 1}, nil
}

type e2eInitiatives struct{}

func (e2eInitiatives) Polling(_ context.Context, id string) (initiatives.Polling, error) {
	if id != e2eInitiativeID {
		return initiatives.Polling{}, initiatives.ErrNotFound
	}

	return initiatives.Polling{ID: id, HouseID: "house-1", Title: "Видеонаблюдение", Stage: "poll"}, nil
}

// e2eQuestions takes questions to the initiator; every user is a member of the house.
type e2eQuestions struct {
	mu    sync.Mutex
	asked []string
}

func (q *e2eQuestions) AskQuestion(_ context.Context, initiativeID, userID, text string) (initiatives.Question, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.asked = append(q.asked, text)

	return initiatives.Question{ID: "q-1", InitiativeID: initiativeID, AskedByUserID: userID, Text: text}, nil
}

func (q *e2eQuestions) AnswerQuestion(context.Context, string, string, string) (initiatives.Question, error) {
	return initiatives.Question{}, nil
}

func (q *e2eQuestions) Question(context.Context, string) (initiatives.Question, error) {
	return initiatives.Question{}, initiatives.ErrQuestionNotFound
}

func (q *e2eQuestions) MayViewInitiatives(context.Context, string, string) (bool, error) {
	return true, nil
}

// e2eNotifier keeps markers in memory and never has jobs: the worker idles.
type e2eNotifier struct {
	mu     sync.Mutex
	marker int64
}

func (n *e2eNotifier) Claim(context.Context, int) ([]notify.ClaimedJob, error) {
	return nil, nil
}
func (n *e2eNotifier) Complete(context.Context, string) error { return nil }
func (n *e2eNotifier) Retry(context.Context, string, error, time.Duration) error {
	return nil
}
func (n *e2eNotifier) ResetStale(context.Context) (int64, error) { return 0, nil }
func (n *e2eNotifier) Purge(context.Context) (int64, error)      { return 0, nil }
func (n *e2eNotifier) LoadMarker(context.Context, int64) (int64, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	return n.marker, nil
}
func (n *e2eNotifier) SaveMarker(_ context.Context, _ int64, marker int64) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.marker = marker

	return nil
}

// fakeMAX emulates the MAX Bot API endpoints the bot uses. It checks what the
// official client really sends: unit tests cover handlers, this test covers wiring.
type fakeMAX struct {
	mu         sync.Mutex
	token      string
	badAuth    bool
	polls      []string // marker query parameter of every GET /updates
	sentTo     []string // "chat:<id>" or "user:<id>" of every POST /messages
	sentBodies []map[string]any
	answers    []string // notification text of every POST /answers
	edits      []string // text of the message updated by every POST /answers
	putEdits   []string // "<message_id>: <text>" of every PUT /messages
	sent       chan struct{}
	answered   chan struct{}
}

func (f *fakeMAX) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if r.Header.Get("Authorization") != f.token {
		f.badAuth = true
	}
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/me":
		_, _ = io.WriteString(w, `{"user_id":555,"first_name":"Дом","username":"dom_test_bot","is_bot":true}`)
	case r.Method == http.MethodGet && r.URL.Path == "/subscriptions":
		_, _ = io.WriteString(w, `{"subscriptions":[]}`)
	case r.Method == http.MethodGet && r.URL.Path == "/updates":
		f.polls = append(f.polls, r.URL.Query().Get("marker"))
		switch len(f.polls) {
		case 1:
			// A neighbour writes in the group chat of the house (the bot must stay
			// silent there), then a user starts the bot by the house link.
			_, _ = io.WriteString(w, `{"marker":5,"updates":[
				{"update_type":"message_created","timestamp":1,"message":{
					"sender":{"user_id":43,"first_name":"Пётр"},
					"recipient":{"chat_id":9,"chat_type":"chat"},
					"body":{"mid":"m-1","seq":1,"text":"/start"}}},
				{"update_type":"bot_started","timestamp":2,
					"chat_id":7,"user":{"user_id":42,"first_name":"Анна"},"payload":"demo-slug_1"}]}`)

			return
		case 2:
			// The user presses «Поддерживаю» on a poll message.
			_, _ = io.WriteString(w, fmt.Sprintf(`{"marker":8,"updates":[
				{"update_type":"message_callback","timestamp":3,
					"callback":{"timestamp":3,"callback_id":"cb-1","payload":"pv:%s:for",
						"user":{"user_id":42,"first_name":"Анна"}}}]}`, e2eInitiativeID))

			return
		case 3:
			// «Есть вопрос», then the question itself as the next message of the dialog.
			_, _ = io.WriteString(w, fmt.Sprintf(`{"marker":10,"updates":[
				{"update_type":"message_callback","timestamp":4,
					"callback":{"timestamp":4,"callback_id":"cb-2","payload":"pv:%s:question",
						"user":{"user_id":42,"first_name":"Анна"}}},
				{"update_type":"message_created","timestamp":5,"message":{
					"sender":{"user_id":42,"first_name":"Анна"},
					"recipient":{"chat_id":7,"chat_type":"dialog"},
					"body":{"mid":"m-2","seq":2,"text":"Кто будет смотреть записи?"}}}]}`, e2eInitiativeID))

			return
		}
		f.mu.Unlock()
		time.Sleep(20 * time.Millisecond) // a real server holds the request until updates or timeout
		f.mu.Lock()
		// An empty response keeps the current position.
		_, _ = fmt.Fprintf(w, `{"marker":%s,"updates":[]}`, f.markerParam(r))
	case r.Method == http.MethodPost && r.URL.Path == "/messages":
		if chat := r.URL.Query().Get("chat_id"); chat != "" {
			f.sentTo = append(f.sentTo, "chat:"+chat)
		} else {
			f.sentTo = append(f.sentTo, "user:"+r.URL.Query().Get("user_id"))
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.sentBodies = append(f.sentBodies, body)
		_, _ = fmt.Fprintf(w, `{"message":{"body":{"mid":"mid-%d"}}}`, len(f.sentBodies))
		select {
		case f.sent <- struct{}{}:
		default:
		}
	case r.Method == http.MethodPut && r.URL.Path == "/messages":
		var body struct {
			Text string `json:"text"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.putEdits = append(f.putEdits, r.URL.Query().Get("message_id")+": "+body.Text)
		_, _ = io.WriteString(w, `{"success":true}`)
	case r.Method == http.MethodPost && r.URL.Path == "/answers":
		var body struct {
			Notification string `json:"notification"`
			Message      *struct {
				Text string `json:"text"`
			} `json:"message"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.answers = append(f.answers, body.Notification)
		if body.Message != nil {
			f.edits = append(f.edits, body.Message.Text)
		}
		_, _ = io.WriteString(w, `{"success":true}`)
		select {
		case f.answered <- struct{}{}:
		default:
		}
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeMAX) markerParam(r *http.Request) string {
	if m := r.URL.Query().Get("marker"); m != "" {
		return m
	}

	return "0"
}

func TestBotEndToEndWithFakeMAX(t *testing.T) {
	fake := &fakeMAX{token: "test-token", sent: make(chan struct{}, 1), answered: make(chan struct{}, 1)}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	votes := &e2eVotes{}
	notifier := &e2eNotifier{}
	questions := &e2eQuestions{}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runBot(ctx, "test-token", botDeps{
			houses:      e2eHouses{},
			houseByID:   e2eHouses{},
			users:       e2eUsers{},
			votes:       votes,
			initiatives: e2eInitiatives{},
			questions:   questions,
			members:     questions,
			notifier:    notifier,
		}, slog.New(slog.NewTextHandler(io.Discard, nil)), maxapi.WithBaseURL(srv.URL))
		close(done)
	}()

	waitFor := func(ch chan struct{}, what string) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s did not happen", what)
		}
	}
	waitFor(fake.sent, "greeting for bot_started")
	waitFor(fake.answered, "answer to the vote callback")
	// The greeting and the prompt for the question; the prompt then turns into the confirmation.
	deadline := time.Now().Add(5 * time.Second)
	for {
		fake.mu.Lock()
		n, edited := len(fake.sentBodies), len(fake.putEdits)
		fake.mu.Unlock()
		if n >= 2 && edited >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d messages sent and %d edited, want the greeting, the prompt and its edit", n, edited)
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond) // let the poller persist the marker
	cancel()
	waitFor(done, "bot stop")

	fake.mu.Lock()
	defer fake.mu.Unlock()
	votes.mu.Lock()
	defer votes.mu.Unlock()

	if fake.badAuth {
		t.Fatal("request without the bot token in Authorization")
	}
	// The greeting goes to the dialog 7, the prompt to the user who pressed; the group chat 9 gets nothing.
	if len(fake.sentTo) != 2 || fake.sentTo[0] != "chat:7" || fake.sentTo[1] != "user:42" {
		t.Fatalf("messages sent to %q, want chat:7 and user:42", fake.sentTo)
	}

	// attachments[0] = inline keyboard, buttons[0][0] = open_app with the house slug.
	atts, _ := fake.sentBodies[0]["attachments"].([]any)
	if len(atts) != 1 {
		t.Fatalf("attachments = %v", fake.sentBodies[0]["attachments"])
	}
	att := atts[0].(map[string]any)
	btn := att["payload"].(map[string]any)["buttons"].([]any)[0].([]any)[0].(map[string]any)
	if att["type"] != "inline_keyboard" || btn["type"] != "open_app" || btn["payload"] != "demo-slug_1" ||
		btn["web_app"] != "dom_test_bot" || btn["contact_id"] != float64(555) {
		t.Fatalf("unexpected keyboard: %v", att)
	}

	// The vote went through the poll module and the user got a weighted receipt.
	if len(votes.cast) != 1 || votes.cast[0].InitiativeID != e2eInitiativeID || votes.cast[0].Choice != poll.ChoiceFor {
		t.Fatalf("votes cast = %+v", votes.cast)
	}
	// Every press is answered: the vote and «Есть вопрос».
	if len(fake.answers) != 2 {
		t.Fatalf("answers = %q", fake.answers)
	}
	if !strings.Contains(fake.answers[0], "Голос учтён") || !strings.Contains(fake.answers[0], "26,15") {
		t.Fatalf("answer = %q, want the weighted receipt", fake.answers[0])
	}
	// The poll message itself is updated with the current choice (решение 68).
	if len(fake.edits) != 1 || !strings.Contains(fake.edits[0], "Ваш голос: «за», 26,15 м²") {
		t.Fatalf("message updates = %q, want the poll message with the current choice", fake.edits)
	}

	// «Есть вопрос»: the prompt goes to the presser in the HTML format with «Отмена»,
	// and the next text of the dialog becomes the question (решение 78).
	prompt := fake.sentBodies[1]
	if prompt["format"] != "html" || !strings.Contains(prompt["text"].(string), "Напишите вопрос") ||
		!strings.Contains(fmt.Sprint(prompt["attachments"]), "qx") {
		t.Fatalf("prompt = %v, want an HTML message with «Отмена»", prompt)
	}
	questions.mu.Lock()
	defer questions.mu.Unlock()
	if len(questions.asked) != 1 || questions.asked[0] != "Кто будет смотреть записи?" {
		t.Fatalf("asked = %q", questions.asked)
	}
	// The prompt (the second message) is edited into the confirmation.
	if len(fake.putEdits) != 1 || !strings.HasPrefix(fake.putEdits[0], "mid-2: ") ||
		!strings.Contains(fake.putEdits[0], "Вопрос отправлен инициатору") {
		t.Fatalf("edits = %q, want the prompt mid-2 turned into the confirmation", fake.putEdits)
	}

	// The marker advanced: every batch was consumed and persisted.
	if notifier.marker != 10 {
		t.Fatalf("saved marker = %d, want 10", notifier.marker)
	}
}

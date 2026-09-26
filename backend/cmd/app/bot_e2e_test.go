package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	maxapi "github.com/max-messenger/max-bot-api-client-go/v2"

	"maxhackathon/backend/internal/registry"
)

type e2eHouses struct{}

func (e2eHouses) HouseBySlug(_ context.Context, slug string) (registry.HouseSummary, error) {
	if slug != "demo-slug_1" {
		return registry.HouseSummary{}, registry.ErrNotFound
	}

	return registry.HouseSummary{InviteSlug: slug, Address: "демо-адрес", IsDemo: true}, nil
}

// fakeMAX emulates the MAX Bot API endpoints the bot uses. It checks what the official
// client really sends: our unit tests cover the handler, this test covers the wiring.
type fakeMAX struct {
	mu       sync.Mutex
	token    string
	badAuth  bool
	polls    []string // marker query parameter of every GET /updates
	sentTo   []string // chat_id of every POST /messages
	sentBody map[string]any
	sent     chan struct{}
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
		if len(f.polls) == 1 {
			// A neighbour writes in the group chat of the house (the bot must stay silent there),
			// then a user starts the bot by the house link.
			_, _ = io.WriteString(w, `{"marker":5,"updates":[
				{"update_type":"message_created","timestamp":1,"message":{
					"sender":{"user_id":43,"first_name":"Пётр"},
					"recipient":{"chat_id":9,"chat_type":"chat"},
					"body":{"mid":"m-1","seq":1,"text":"/start"}}},
				{"update_type":"bot_started","timestamp":2,
					"chat_id":7,"user":{"user_id":42,"first_name":"Анна"},"payload":"demo-slug_1"}]}`)

			return
		}
		f.mu.Unlock()
		time.Sleep(20 * time.Millisecond) // a real server holds the request until updates or timeout
		f.mu.Lock()
		_, _ = io.WriteString(w, `{"marker":5,"updates":[]}`)
	case r.Method == http.MethodPost && r.URL.Path == "/messages":
		f.sentTo = append(f.sentTo, r.URL.Query().Get("chat_id"))
		_ = json.NewDecoder(r.Body).Decode(&f.sentBody)
		_, _ = io.WriteString(w, `{"message":{}}`)
		select {
		case f.sent <- struct{}{}:
		default:
		}
	default:
		http.NotFound(w, r)
	}
}

func TestBotEndToEndWithFakeMAX(t *testing.T) {
	fake := &fakeMAX{token: "test-token", sent: make(chan struct{}, 1)}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runBot(ctx, "test-token", e2eHouses{}, slog.New(slog.NewTextHandler(io.Discard, nil)), maxapi.WithBaseURL(srv.URL))
		close(done)
	}()

	select {
	case <-fake.sent:
	case <-time.After(5 * time.Second):
		t.Fatal("bot did not answer bot_started")
	}
	time.Sleep(100 * time.Millisecond) // let the poller make a few more calls
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("bot did not stop after context cancel")
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()

	if fake.badAuth {
		t.Fatal("request without the bot token in Authorization")
	}
	if len(fake.sentTo) != 1 || fake.sentTo[0] != "7" {
		t.Fatalf("answers sent to chats %q, want only the dialog 7 (the group chat 9 gets nothing)", fake.sentTo)
	}

	// attachments[0] = inline keyboard, buttons[0][0] = open_app with the house slug.
	atts, _ := fake.sentBody["attachments"].([]any)
	if len(atts) != 1 {
		t.Fatalf("attachments = %v", fake.sentBody["attachments"])
	}
	att := atts[0].(map[string]any)
	btn := att["payload"].(map[string]any)["buttons"].([]any)[0].([]any)[0].(map[string]any)
	if att["type"] != "inline_keyboard" || btn["type"] != "open_app" || btn["payload"] != "demo-slug_1" ||
		btn["web_app"] != "dom_test_bot" || btn["contact_id"] != float64(555) {
		t.Fatalf("unexpected keyboard: %v", att)
	}

	if len(fake.polls) < 2 || (fake.polls[0] != "" && fake.polls[0] != "0") || fake.polls[1] != "5" {
		t.Fatalf("markers of /updates calls = %v, want first empty/0 then 5", fake.polls)
	}
}

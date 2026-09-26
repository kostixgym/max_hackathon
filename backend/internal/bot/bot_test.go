package bot

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	maxapi "github.com/max-messenger/max-bot-api-client-go/v2"
	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/registry"
)

type sentMessage struct {
	body model.NewMessageBody
}

type fakeMessages struct{ sent []sentMessage }

func (f *fakeMessages) Send(_ context.Context, msg *maxapi.Message) (model.SendMessageResult, error) {
	f.sent = append(f.sent, sentMessage{body: msg.MessageBody()})

	return model.SendMessageResult{}, nil
}

type fakeHouses struct{}

func (fakeHouses) HouseBySlug(_ context.Context, slug string) (registry.HouseSummary, error) {
	if slug != "demo-slug_1" {
		return registry.HouseSummary{}, registry.ErrNotFound
	}

	return registry.HouseSummary{InviteSlug: slug, Address: "г. Казань, ул. Демонстрационная, д. 1", IsDemo: true}, nil
}

func newBot() (*Bot, *fakeMessages) {
	msgs := &fakeMessages{}

	return &Bot{
		Messages: msgs,
		Houses:   fakeHouses{},
		Me:       Identity{UserID: 555, Username: "dom_test_bot"},
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, msgs
}

// openApp returns the single open_app button of the message.
func openApp(t *testing.T, body model.NewMessageBody) *model.Button {
	t.Helper()
	if len(body.Attachments) != 1 || body.Attachments[0].Type != model.AttachInlineKeyboard {
		t.Fatalf("expected one inline keyboard, got %+v", body.Attachments)
	}
	btn := body.Attachments[0].Payload.Buttons[0][0]
	if btn.Type != model.ButtonOpenApp || btn.ContactID != 555 || btn.WebApp != "dom_test_bot" {
		t.Fatalf("expected open_app button of this bot, got %+v", btn)
	}

	return btn
}

func TestBotStartedWithHouseLink(t *testing.T) {
	b, msgs := newBot()
	b.Handle(context.Background(), model.Update{UpdateType: model.UpdateBotStarted, UserID: 42, ChatID: 7, Payload: "demo-slug_1"})

	if len(msgs.sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(msgs.sent))
	}
	body := msgs.sent[0].body
	if !strings.Contains(body.Text, "ул. Демонстрационная") || !strings.Contains(body.Text, "демо-дом") {
		t.Fatalf("greeting must name the house and mark the demo: %q", body.Text)
	}
	if btn := openApp(t, body); btn.Payload != "demo-slug_1" {
		t.Fatalf("house slug must be passed to the mini-app as start_param, got %q", btn.Payload)
	}
}

func TestStartWithoutOrWithUnknownLink(t *testing.T) {
	cases := map[string]model.Update{
		"bot_started without payload": {UpdateType: model.UpdateBotStarted, UserID: 42, ChatID: 7},
		"unknown slug":                {UpdateType: model.UpdateBotStarted, UserID: 42, ChatID: 7, Payload: "no-such-house"},
		"payload not allowed by MAX":  {UpdateType: model.UpdateBotStarted, UserID: 42, ChatID: 7, Payload: "bad slug!"},
		"/start command": {
			UpdateType: model.UpdateMessageCreated, UserID: 42, ChatID: 7,
			Message: &model.MessageUpdate{Body: model.MessageBody{Text: "/start"}},
		},
	}
	for name, u := range cases {
		t.Run(name, func(t *testing.T) {
			b, msgs := newBot()
			b.Handle(context.Background(), u)
			if len(msgs.sent) != 1 {
				t.Fatalf("sent %d messages, want 1", len(msgs.sent))
			}
			body := msgs.sent[0].body
			if !strings.Contains(body.Text, "ссылку или QR-код") {
				t.Fatalf("generic greeting expected: %q", body.Text)
			}
			if btn := openApp(t, body); btn.Payload != "" {
				t.Fatalf("no payload expected, got %q", btn.Payload)
			}
		})
	}
}

func TestStartCommandWithSlug(t *testing.T) {
	b, msgs := newBot()
	b.Handle(context.Background(), model.Update{
		UpdateType: model.UpdateMessageCreated, UserID: 42, ChatID: 7,
		Message: &model.MessageUpdate{Body: model.MessageBody{Text: "/start demo-slug_1"}},
	})
	if btn := openApp(t, msgs.sent[0].body); btn.Payload != "demo-slug_1" {
		t.Fatalf("payload = %q", btn.Payload)
	}
}

func TestOtherMessagesGetHelp(t *testing.T) {
	b, msgs := newBot()
	b.Handle(context.Background(), model.Update{
		UpdateType: model.UpdateMessageCreated, UserID: 42, ChatID: 7,
		Message: &model.MessageUpdate{Body: model.MessageBody{Text: "привет"}},
	})
	if len(msgs.sent) != 1 || !strings.Contains(msgs.sent[0].body.Text, "Все действия") {
		t.Fatalf("help expected, got %+v", msgs.sent)
	}
}

func TestIgnoresBotsAndOtherUpdates(t *testing.T) {
	b, msgs := newBot()
	b.Handle(context.Background(), model.Update{
		UpdateType: model.UpdateMessageCreated, ChatID: 7,
		Message: &model.MessageUpdate{Sender: model.Sender{IsBot: true}, Body: model.MessageBody{Text: "/start"}},
	})
	b.Handle(context.Background(), model.Update{UpdateType: model.UpdateDialogMuted, ChatID: 7})
	if len(msgs.sent) != 0 {
		t.Fatalf("sent %d messages, want 0", len(msgs.sent))
	}
}

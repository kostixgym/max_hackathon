// Package bot handles chat updates: greets the user and opens the mini-app.
// The chat is for notifications and short actions; complex screens live in the
// mini-app (docs/02). Voting buttons and reminders come in stage 1.
//
// The bot talks only in private dialogs (decision 44). In a group chat of the house
// it stays silent, otherwise it would answer every message of the neighbours.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"

	maxapi "github.com/max-messenger/max-bot-api-client-go/v2"
	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/registry"
)

// Messages sends messages (the MAX client).
type Messages interface {
	Send(ctx context.Context, msg *maxapi.Message) (model.SendMessageResult, error)
}

// Houses finds a house by its invite slug (the registry module).
type Houses interface {
	HouseBySlug(ctx context.Context, slug string) (registry.HouseSummary, error)
}

// Identity is the bot itself: the open_app button must name the bot the mini-app is wired to.
type Identity struct {
	UserID   int64
	Username string
}

// Bot answers chat updates.
type Bot struct {
	Messages Messages
	Houses   Houses
	Me       Identity
	Log      *slog.Logger
}

// openAppPayload is what MAX accepts in the payload of an open_app button;
// it becomes start_param of the mini-app.
var openAppPayload = regexp.MustCompile(`^[\w-]{1,512}$`)

// Handle implements maxbot.Handler.
func (b *Bot) Handle(ctx context.Context, u model.Update) {
	switch u.UpdateType {
	case model.UpdateBotStarted:
		// «Начать» in a new dialog; a deep link max.ru/<bot>?start=<slug> puts the slug into Payload.
		b.start(ctx, u, u.Payload)
	case model.UpdateMessageCreated:
		msg := u.GetMessage()
		if msg.Sender.IsBot || msg.Recipient.ChatType != model.ChatTypeDialog {
			return
		}
		cmd := u.GetCommand()
		if cmd.Command == "/start" {
			b.start(ctx, u, cmd.RemainingText)

			return
		}
		b.help(ctx, u)
	}
}

func (b *Bot) start(ctx context.Context, u model.Update, slug string) {
	if slug != "" && openAppPayload.MatchString(slug) {
		house, err := b.Houses.HouseBySlug(ctx, slug)
		switch {
		case err == nil:
			b.send(ctx, u, houseGreeting(house), openAppButton(b.Me, "Открыть приложение", house.InviteSlug))

			return
		case !errors.Is(err, registry.ErrNotFound):
			b.Log.Error("bot: house by slug", "err", err)
		}
	}

	b.send(ctx, u, genericGreeting, openAppButton(b.Me, "Открыть приложение", ""))
}

func (b *Bot) help(ctx context.Context, u model.Update) {
	b.send(ctx, u, helpText, openAppButton(b.Me, "Открыть приложение", ""))
}

func (b *Bot) send(ctx context.Context, u model.Update, text string, kb *model.Keyboard) {
	msg := maxapi.NewMessage().SetText(text).AddKeyboard(kb)
	if u.ChatID != 0 {
		msg.SetChat(u.ChatID)
	} else {
		msg.SetUser(u.UserID)
	}

	if _, err := b.Messages.Send(ctx, msg); err != nil {
		b.Log.Error("bot: send message", "err", err, "type", u.UpdateType)
	}
}

// openAppButton opens the mini-app of this bot. The payload (house invite slug)
// reaches the mini-app as start_param, so the house is selected without a search.
func openAppButton(me Identity, text, payload string) *model.Keyboard {
	kb := model.NewKeyboard()
	kb.AddRow().AddButton(model.Button{
		Type:      model.ButtonOpenApp,
		Text:      text,
		WebApp:    me.Username,
		ContactID: me.UserID,
		Payload:   payload,
	})

	return kb
}

const genericGreeting = `Здравствуйте! Здесь соседи решают вопросы дома вместе: камеры, домофон, ремонт подъезда и другие.

Приложение подскажет, как правильно провести общее собрание собственников, соберёт поддержку соседей и посчитает голоса по площади.

Чтобы попасть в свой дом, откройте ссылку или QR-код от вашей управляющей компании. Или нажмите кнопку ниже.`

const helpText = `Все действия — в приложении: выбор дома, инициативы, опросы и голосование. Нажмите кнопку ниже, чтобы открыть его.`

func houseGreeting(h registry.HouseSummary) string {
	text := fmt.Sprintf(`Здравствуйте! Это приложение вашего дома:
%s

Здесь можно предложить улучшение для дома, узнать мнение соседей и довести идею до законного решения общего собрания.

Нажмите «Открыть приложение», чтобы начать.`, h.Address)
	if h.IsDemo {
		text += "\n\nЭто демо-дом: все данные в нём синтетические."
	}

	return text
}

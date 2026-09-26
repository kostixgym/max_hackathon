// Package bot handles chat updates: greets the user and opens the mini-app, takes
// poll votes and questions to the initiator. The chat is for notifications and short
// actions; complex screens live in the mini-app (docs/02).
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
	"strings"
	"time"

	maxapi "github.com/max-messenger/max-bot-api-client-go/v2"
	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/registry"
)

// Messages sends and edits messages (the MAX client).
type Messages interface {
	Send(ctx context.Context, msg *maxapi.Message) (model.SendMessageResult, error)
	EditMessage(ctx context.Context, messageID string, body model.NewMessageBody) (model.SimpleQueryResult, error)
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

	// Poll voting deps (stage 1). Callbacks are ignored when they are not wired.
	Users   Users
	Votes   Votes
	Answers Answers
	// Initiatives and HousesByID rebuild the poll message after a press, so it shows
	// the voter's current choice (решение 68). Without them only a notification is shown.
	Initiatives PollingReader
	HousesByID  HouseReader
	// Progress adds the support in м² to the poll message; optional.
	Progress PollProgress

	// Questions to the initiator (решение 78). Without them «Есть вопрос» only says
	// that questions are not available.
	Questions        Questions
	Members          Members
	InitiativeReader InitiativeReader

	// DevMode enables /id: it tells the user their MAX id, which the API of the
	// development mode accepts in X-Dev-User-Id. Until the mini-app is ready this is
	// the only way to act in the API as a real MAX account. The id is not logged.
	DevMode bool

	// Now is the clock of the bot; nil means time.Now.
	Now func() time.Time

	dialogs dialogs
}

func (b *Bot) now() time.Time { return clock(b.Now) }

// clock reads a replaceable clock: nil means time.Now.
func clock(now func() time.Time) time.Time {
	if now != nil {
		return now()
	}

	return time.Now()
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
	case model.UpdateMessageCallback:
		b.callback(ctx, u)
	case model.UpdateMessageCreated:
		msg := u.GetMessage()
		if msg.Sender.IsBot || msg.Recipient.ChatType != model.ChatTypeDialog {
			return
		}
		cmd := u.GetCommand()
		if strings.HasPrefix(msg.Body.Text, "/") {
			// A command ends a pending question or answer: it is not the awaited text.
			b.dialogs.clear(u.UserID)
		} else if b.dialogText(ctx, u) {
			return
		}
		switch {
		case cmd.Command == "/start":
			b.start(ctx, u, cmd.RemainingText)
		case cmd.Command == "/id" && b.DevMode:
			b.send(ctx, u, fmt.Sprintf("Ваш MAX id: %d\n\nКоманда работает только в режиме разработки (DEV_MODE).", u.UserID), nil)
		default:
			b.help(ctx, u)
		}
	}
}

func (b *Bot) start(ctx context.Context, u model.Update, slug string) {
	if id, ok := strings.CutPrefix(slug, pollStartPrefix); ok {
		if b.openPoll(ctx, u, id) {
			return
		}
		slug = "" // no such poll: the generic greeting
	}
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

// openPoll answers a poll link from the initiator (решение 78): a verified member of
// the house gets the poll message, anyone else only the title and how to join
// (решение 19). It reports false when there is no such poll to show.
func (b *Bot) openPoll(ctx context.Context, u model.Update, initiativeID string) bool {
	if b.Users == nil || b.Initiatives == nil || b.HousesByID == nil || b.Members == nil || u.UserID <= 0 {
		return false
	}
	initiative, err := b.Initiatives.Polling(ctx, initiativeID)
	if err != nil {
		if !errors.Is(err, initiatives.ErrNotFound) {
			b.Log.Error("bot: initiative of a poll link", "err", err)
		}

		return false
	}
	if initiative.Stage == initiatives.StageDraft {
		return false // a draft is seen by its initiator only (решение 76)
	}
	house, err := b.HousesByID.House(ctx, initiative.HouseID)
	if err != nil {
		b.Log.Error("bot: house of a poll link", "err", err)

		return false
	}
	user, err := b.Users.EnsureUser(ctx, u.UserID)
	if err != nil {
		b.Log.Error("bot: ensure user of a poll link", "err", err)

		return false
	}
	member, err := b.Members.MayViewInitiatives(ctx, user.ID, house.ID)
	if err != nil {
		b.Log.Error("bot: check member of a poll link", "err", err)

		return false
	}
	if !member {
		b.sendHTML(ctx, u.UserID, fmt.Sprintf("<b>Соседи обсуждают «%s»</b>\n%s\n\n"+
			"Голосовать и задавать вопросы могут подтверждённые жители дома. Откройте приложение и подтвердите квартиру.",
			esc(initiative.Title), esc(house.Address)), openAppButton(b.Me, "Открыть приложение", house.InviteSlug))

		return true
	}

	view := pollView{initiative: initiative, house: house, now: b.now(), forInitiator: initiative.IsInitiator(user.ID)}
	if b.Progress != nil {
		if progress, err := b.Progress.Progress(ctx, initiative.ID); err == nil {
			view.progress = &progress
		}
	}
	text, kb := view.render(b.Me)
	b.sendHTML(ctx, u.UserID, text, kb)

	return true
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

// sendHTML writes to the user by MAX id; the text is HTML, parts from users escaped.
func (b *Bot) sendHTML(ctx context.Context, maxUserID int64, text string, kb *model.Keyboard) {
	msg := maxapi.NewMessage().SetUser(maxUserID).SetText(text).SetFormat(model.FormatHTML).AddKeyboard(kb)
	if _, err := b.Messages.Send(ctx, msg); err != nil {
		b.Log.Error("bot: send message", "err", err)
	}
}

// notice answers a press with a text the user must see. MAX does not show the
// notification of a callback answer (проверка 0.6), so the text also goes as a message.
func (b *Bot) notice(ctx context.Context, cb *model.Callback, text string) {
	b.answer(ctx, cb.CallbackID, text)
	b.sendHTML(ctx, cb.User.UserID, esc(text), nil)
}

// openAppButton is a keyboard with the single button that opens the mini-app.
func openAppButton(me Identity, text, payload string) *model.Keyboard {
	kb := model.NewKeyboard()
	kb.AddRow().AddButton(appButton(me, text, payload))

	return kb
}

// appButton opens the mini-app of this bot. The payload (house invite slug) reaches
// the mini-app as start_param, so the house is selected without a search. A payload
// MAX would reject is dropped: the button then opens the app without a house.
func appButton(me Identity, text, payload string) model.Button {
	if !openAppPayload.MatchString(payload) {
		payload = ""
	}

	return model.Button{
		Type:      model.ButtonOpenApp,
		Text:      text,
		WebApp:    me.Username,
		ContactID: me.UserID,
		Payload:   payload,
	}
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

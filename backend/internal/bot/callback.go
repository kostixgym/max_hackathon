package bot

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	maxapi "github.com/max-messenger/max-bot-api-client-go/v2"
	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/poll"
	"maxhackathon/backend/internal/registry"
)

// Chat voting: the poll message carries callback buttons, the press goes to the
// poll module and the answer returns through POST /answers (docs/04, решение 68:
// no stored message id is needed to react to a press).

// Votes casts poll votes (the poll module).
type Votes interface {
	CastVote(ctx context.Context, in poll.CastInput) (poll.CastResult, error)
}

// Users finds the internal user by MAX id (the access module).
type Users interface {
	EnsureUser(ctx context.Context, maxUserID int64) (access.User, error)
}

// Answers replies to a callback press (the MAX client).
type Answers interface {
	AnswerOnCallback(ctx context.Context, callbackID string, answer model.CallbackAnswer) (model.SimpleQueryResult, error)
}

// votePayloadPrefix marks the payloads of poll voting buttons: pv:<initiative>:<choice>.
const votePayloadPrefix = "pv"

// callback handles a button press: a vote or «Есть вопрос» on a poll message,
// «Ответить» on a relayed question, «Отмена» on a prompt.
func (b *Bot) callback(ctx context.Context, u model.Update) {
	cb := u.Callback
	if cb == nil || cb.User.UserID <= 0 {
		// Without the presser there is nobody to count the vote for.
		return
	}
	if b.Users == nil || b.Answers == nil {
		b.Log.Warn("bot: callback came with deps not wired", "payload", cb.Payload)

		return
	}

	switch payload := cb.Payload; {
	case payload == cancelPayload:
		b.cancelDialog(ctx, cb)

		return
	case strings.HasPrefix(payload, answerPayloadPrefix+":"):
		b.startAnswer(ctx, cb, strings.TrimPrefix(payload, answerPayloadPrefix+":"))

		return
	}

	initiativeID, action, ok := parseVotePayload(cb.Payload)
	if !ok {
		// Not ours (future button kinds); silently ignore.
		return
	}
	if action == "question" {
		b.startQuestion(ctx, cb, initiativeID)

		return
	}
	if b.Votes == nil {
		b.Log.Warn("bot: vote came with the poll module not wired")

		return
	}

	choice := poll.ChoiceFor
	if action == "against" {
		choice = poll.ChoiceAgainst
	}

	user, err := b.Users.EnsureUser(ctx, cb.User.UserID)
	if err != nil {
		b.Log.Error("bot: ensure user for vote", "err", err)
		b.notice(ctx, cb, "Не получилось учесть голос, попробуйте ещё раз")

		return
	}

	result, err := b.Votes.CastVote(ctx, poll.CastInput{
		InitiativeID: initiativeID,
		UserID:       user.ID,
		Choice:       choice,
	})
	switch {
	case errors.Is(err, poll.ErrNotOwner):
		b.notice(ctx, cb, "Голосуют только подтверждённые собственники. Откройте приложение и подтвердите квартиру")
	case errors.Is(err, poll.ErrPollClosed):
		b.answerWithMessage(ctx, cb.CallbackID, initiativeID, user.ID, nil, "Опрос завершён")
	case errors.Is(err, poll.ErrNoWeight):
		b.notice(ctx, cb, "Вашей записи нет в версии реестра этого опроса. Откройте приложение и отправьте «Данные неверны»")
	case err != nil:
		b.Log.Error("bot: cast vote", "err", err, "initiative", initiativeID)
		b.notice(ctx, cb, "Не получилось учесть голос, попробуйте ещё раз")
	default:
		b.answerWithMessage(ctx, cb.CallbackID, initiativeID, user.ID, &result, voteAcceptedText(result))
	}
}

func (b *Bot) answer(ctx context.Context, callbackID, notification string) {
	b.sendAnswer(ctx, callbackID, model.CallbackAnswer{Notification: &notification})
}

// answerWithMessage shows the notification and rebuilds the poll message: after a
// vote it shows the voter's choice, after the end of the poll the voting buttons are
// gone (решение 68: POST /answers updates the message, no stored message id is needed).
// If the message cannot be rebuilt, the notification alone is shown.
func (b *Bot) answerWithMessage(ctx context.Context, callbackID, initiativeID, viewerID string, vote *poll.CastResult, notification string) {
	answer := model.CallbackAnswer{Notification: &notification}
	if body, ok := b.pollMessageBody(ctx, initiativeID, viewerID, vote); ok {
		answer.Message = &body
	}
	b.sendAnswer(ctx, callbackID, answer)
}

// pollMessageBody renders the poll message for the viewer (a user id of ours).
func (b *Bot) pollMessageBody(ctx context.Context, initiativeID, viewerID string, vote *poll.CastResult) (model.NewMessageBody, bool) {
	if b.Initiatives == nil || b.HousesByID == nil {
		return model.NewMessageBody{}, false
	}
	initiative, err := b.Initiatives.Polling(ctx, initiativeID)
	if err != nil {
		if !errors.Is(err, initiatives.ErrNotFound) {
			b.Log.Error("bot: initiative for the poll message", "err", err, "initiative", initiativeID)
		}

		return model.NewMessageBody{}, false
	}
	house, err := b.HousesByID.House(ctx, initiative.HouseID)
	if err != nil {
		b.Log.Error("bot: house for the poll message", "err", err, "initiative", initiativeID)

		return model.NewMessageBody{}, false
	}

	view := pollView{initiative: initiative, house: house, vote: vote, now: b.now(),
		forInitiator: initiative.IsInitiator(viewerID)}
	if b.Progress != nil {
		// Without the support line the message still shows the vote.
		if progress, err := b.Progress.Progress(ctx, initiativeID); err == nil {
			view.progress = &progress
		}
	}
	text, kb := view.render(b.Me)

	return maxapi.NewMessage().SetText(text).SetFormat(model.FormatHTML).AddKeyboard(kb).MessageBody(), true
}

func (b *Bot) sendAnswer(ctx context.Context, callbackID string, answer model.CallbackAnswer) {
	if _, err := b.Answers.AnswerOnCallback(ctx, callbackID, answer); err != nil {
		b.Log.Error("bot: answer on callback", "err", err, "callback_id", callbackID)
	}
}

func voteAcceptedText(result poll.CastResult) string {
	weight := new(big.Rat).SetFrac64(result.WeightNum, result.WeightDen*100) // сотые м² → м²
	word := "«за»"
	if result.Choice == poll.ChoiceAgainst {
		word = "«против»"
	}

	text := fmt.Sprintf("Голос учтён: %s м² %s (кв. %s).", registry.FormatM2(weight), word, result.PremiseNumber)
	if result.Choice == poll.ChoiceFor {
		text += " Можно изменить до конца опроса."
	}

	return text
}

// parseVotePayload splits pv:<initiative-id>:<action>. Anything else is not ours.
func parseVotePayload(payload string) (initiativeID, action string, ok bool) {
	parts := strings.Split(payload, ":")
	if len(parts) != 3 || parts[0] != votePayloadPrefix {
		return "", "", false
	}
	if parts[1] == "" || (parts[2] != "for" && parts[2] != "against" && parts[2] != "question") {
		return "", "", false
	}

	return parts[1], parts[2], true
}

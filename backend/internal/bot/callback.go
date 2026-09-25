package bot

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/access"
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

// callback handles a button press on a poll message.
func (b *Bot) callback(ctx context.Context, u model.Update) {
	cb := u.Callback
	if cb == nil {
		return
	}

	initiativeID, action, ok := parseVotePayload(cb.Payload)
	if !ok {
		// Not ours (future button kinds); silently ignore.
		return
	}

	if b.Users == nil || b.Votes == nil || b.Answers == nil {
		b.Log.Warn("bot: callback came with poll deps not wired", "payload", cb.Payload)

		return
	}

	if action == "question" {
		b.answer(ctx, cb.CallbackID, "Вопросы можно задать на экране инициативы в приложении")
		// The mini-app FAQ (InitiativeQuestion) arrives with the initiative screen.

		return
	}

	choice := poll.ChoiceFor
	if action == "against" {
		choice = poll.ChoiceAgainst
	}

	user, err := b.Users.EnsureUser(ctx, cb.User.UserID)
	if err != nil {
		b.Log.Error("bot: ensure user for vote", "err", err)
		b.answer(ctx, cb.CallbackID, "Не получилось учесть голос, попробуйте ещё раз")

		return
	}

	result, err := b.Votes.CastVote(ctx, poll.CastInput{
		InitiativeID: initiativeID,
		UserID:       user.ID,
		Choice:       choice,
	})
	switch {
	case errors.Is(err, poll.ErrNotOwner):
		b.answer(ctx, cb.CallbackID, "Голосуют только подтверждённые собственники. Откройте приложение и подтвердите квартиру")
	case errors.Is(err, poll.ErrPollClosed):
		b.answer(ctx, cb.CallbackID, "Опрос завершён")
	case errors.Is(err, poll.ErrNoWeight):
		b.answer(ctx, cb.CallbackID, "Вашей записи нет в версии реестра этого опроса. Откройте приложение и отправьте «Данные неверны»")
	case err != nil:
		b.Log.Error("bot: cast vote", "err", err, "initiative", initiativeID)
		b.answer(ctx, cb.CallbackID, "Не получилось учесть голос, попробуйте ещё раз")
	default:
		b.answer(ctx, cb.CallbackID, voteAcceptedText(result))
	}
}

func (b *Bot) answer(ctx context.Context, callbackID, notification string) {
	if _, err := b.Answers.AnswerOnCallback(ctx, callbackID, model.CallbackAnswer{
		Notification: &notification,
	}); err != nil {
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

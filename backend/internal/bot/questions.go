package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	maxapi "github.com/max-messenger/max-bot-api-client-go/v2"
	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/initiatives"
)

// Questions to the initiator in the chat (docs/04, решение 78): «Есть вопрос» → the
// bot asks for the text → the question goes to the initiator without the name of the
// asker → «Ответить» → the answer comes back to the asker. Visible reactions are
// messages: MAX does not show the notification of a callback answer (проверка 0.6).

// Questions stores questions and answers (the initiatives module).
type Questions interface {
	AskQuestion(ctx context.Context, initiativeID, userID, text string) (initiatives.Question, error)
	AnswerQuestion(ctx context.Context, questionID, userID, text string) (initiatives.Question, error)
	Question(ctx context.Context, id string) (initiatives.Question, error)
}

// Members tells whether a user is a verified member of the house (the access module).
type Members interface {
	MayViewInitiatives(ctx context.Context, userID, houseID string) (bool, error)
}

// Accounts finds the MAX id of a user to write to (the access module).
type Accounts interface {
	MaxUserID(ctx context.Context, userID string) (int64, error)
}

// InitiativeReader reads an initiative (the initiatives module).
type InitiativeReader interface {
	Get(ctx context.Context, id string) (initiatives.Initiative, error)
}

const (
	answerPayloadPrefix = "qa" // qa:<question id> — «Ответить» on a relayed question
	cancelPayload       = "qx" // «Отмена» on a prompt
)

// startQuestion handles «Есть вопрос»: the next text of the user becomes the question.
func (b *Bot) startQuestion(ctx context.Context, cb *model.Callback, initiativeID string) {
	if b.Questions == nil || b.Members == nil || b.Initiatives == nil {
		b.Log.Warn("bot: question came with the questions deps not wired")
		b.notice(ctx, cb, "Вопросы пока недоступны")

		return
	}
	user, err := b.Users.EnsureUser(ctx, cb.User.UserID)
	if err != nil {
		b.Log.Error("bot: ensure user for a question", "err", err)
		b.notice(ctx, cb, "Не получилось, попробуйте ещё раз")

		return
	}
	initiative, err := b.Initiatives.Polling(ctx, initiativeID)
	if errors.Is(err, initiatives.ErrNotFound) {
		b.notice(ctx, cb, "Инициатива не найдена")

		return
	}
	if err != nil {
		b.Log.Error("bot: initiative for a question", "err", err, "initiative", initiativeID)
		b.notice(ctx, cb, "Не получилось, попробуйте ещё раз")

		return
	}
	if !initiative.TakesQuestions() {
		b.notice(ctx, cb, "Инициатива завершена, вопросы по ней больше не принимаются")

		return
	}
	member, err := b.Members.MayViewInitiatives(ctx, user.ID, initiative.HouseID)
	if err != nil {
		b.Log.Error("bot: check member for a question", "err", err)
		b.notice(ctx, cb, "Не получилось, попробуйте ещё раз")

		return
	}
	if !member {
		b.notice(ctx, cb, "Задавать вопросы могут подтверждённые жители дома. Откройте приложение и подтвердите квартиру")

		return
	}
	if initiative.IsInitiator(user.ID) {
		// An old message of the initiator may still carry the button.
		b.notice(ctx, cb, "Это ваша инициатива: вопросы задают соседи, а вы отвечаете на них здесь, в чате")

		return
	}

	b.answer(ctx, cb.CallbackID, "Напишите вопрос следующим сообщением")
	b.prompt(ctx, cb.User.UserID, dialogQuestion, initiative.ID, fmt.Sprintf(
		"Напишите вопрос инициатору <b>«%s»</b> одним сообщением.\n\n"+
			"Кто спросил, он не увидит. Ответ придёт сюда же.", esc(initiative.Title)))
}

// startAnswer handles «Ответить» on a relayed question: only the initiator answers,
// once.
func (b *Bot) startAnswer(ctx context.Context, cb *model.Callback, questionID string) {
	if b.Questions == nil || b.InitiativeReader == nil {
		b.Log.Warn("bot: answer came with the questions deps not wired")
		b.notice(ctx, cb, "Ответы пока недоступны")

		return
	}
	user, err := b.Users.EnsureUser(ctx, cb.User.UserID)
	if err != nil {
		b.Log.Error("bot: ensure user for an answer", "err", err)
		b.notice(ctx, cb, "Не получилось, попробуйте ещё раз")

		return
	}
	q, err := b.Questions.Question(ctx, questionID)
	if errors.Is(err, initiatives.ErrQuestionNotFound) {
		b.notice(ctx, cb, "Вопрос не найден: возможно, сосед удалил аккаунт")

		return
	}
	if err != nil {
		b.Log.Error("bot: question for an answer", "err", err)
		b.notice(ctx, cb, "Не получилось, попробуйте ещё раз")

		return
	}
	initiative, err := b.InitiativeReader.Get(ctx, q.InitiativeID)
	if errors.Is(err, initiatives.ErrNotFound) {
		b.notice(ctx, cb, "Инициатива не найдена")

		return
	}
	if err != nil {
		b.Log.Error("bot: initiative for an answer", "err", err)
		b.notice(ctx, cb, "Не получилось, попробуйте ещё раз")

		return
	}
	switch {
	case initiative.InitiatorUserID == nil || *initiative.InitiatorUserID != user.ID:
		b.notice(ctx, cb, "Ответить на вопрос может только инициатор")

		return
	case q.AnsweredAt != nil:
		b.notice(ctx, cb, "На этот вопрос вы уже ответили")

		return
	}

	b.answer(ctx, cb.CallbackID, "Напишите ответ следующим сообщением")
	b.prompt(ctx, cb.User.UserID, dialogAnswer, q.ID, fmt.Sprintf(
		"Напишите ответ соседу одним сообщением.\n\n<i>Вопрос:</i>\n<blockquote>%s</blockquote>", esc(q.Text)))
}

// prompt asks the user for a text and waits for it; the prompt message turns into
// the confirmation when the text is sent.
func (b *Bot) prompt(ctx context.Context, maxUserID int64, kind, ref, text string) {
	msg := maxapi.NewMessage().SetUser(maxUserID).SetText(text).SetFormat(model.FormatHTML).AddKeyboard(cancelKeyboard())
	res, err := b.Messages.Send(ctx, msg)
	if err != nil {
		b.Log.Error("bot: send prompt", "err", err)

		return
	}
	b.dialogs.set(maxUserID, kind, ref, res.Message.Body.Mid, b.now())
}

// done turns the prompt into the confirmation without buttons, so no stale «Отмена»
// stays in the chat. If the prompt cannot be edited, the confirmation is a new message.
func (b *Bot) done(ctx context.Context, maxUserID int64, d dialog, text string) {
	if d.prompt != "" {
		body := maxapi.NewMessage().SetText(text).SetFormat(model.FormatHTML).MessageBody()
		body.Attachments = []model.Attachment{} // an empty list removes the keyboard
		if res, err := b.Messages.EditMessage(ctx, d.prompt, body); err == nil && res.Success {
			return
		}
	}
	b.sendHTML(ctx, maxUserID, text, nil)
}

// cancelDialog handles «Отмена» on a prompt and removes the button from it.
func (b *Bot) cancelDialog(ctx context.Context, cb *model.Callback) {
	text := "Отменено."
	if !b.dialogs.clear(cb.User.UserID) {
		text = "Уже неактуально: сообщение отправлено или время ожидания вышло."
	}
	body := maxapi.NewMessage().SetText(text).MessageBody()
	body.Attachments = []model.Attachment{} // an empty list removes the keyboard, null would keep it
	b.sendAnswer(ctx, cb.CallbackID, model.CallbackAnswer{Message: &body, Notification: &text})
}

// dialogText handles the text that a pending dialog waits for. It reports false when
// nothing was pending, and the text is then an ordinary message.
func (b *Bot) dialogText(ctx context.Context, u model.Update) bool {
	d, ok := b.dialogs.take(u.UserID, b.now())
	if !ok {
		return false
	}
	text := u.GetMessage().Body.Text
	if text == "" {
		// A sticker or a photo: keep waiting for the text.
		b.prompt(ctx, u.UserID, d.kind, d.ref, "Пришлите текст одним сообщением или нажмите «Отмена».")

		return true
	}
	user, err := b.Users.EnsureUser(ctx, u.UserID)
	if err != nil {
		b.Log.Error("bot: ensure user for a dialog", "err", err)
		b.sendHTML(ctx, u.UserID, "Не получилось отправить, попробуйте ещё раз.", nil)

		return true
	}

	switch d.kind {
	case dialogQuestion:
		b.askQuestion(ctx, u.UserID, user.ID, d, text)
	case dialogAnswer:
		b.answerQuestion(ctx, u.UserID, user.ID, d, text)
	}

	return true
}

func (b *Bot) askQuestion(ctx context.Context, maxUserID int64, userID string, d dialog, text string) {
	initiativeID := d.ref
	// Membership is checked again: the link could be revoked while the user was typing.
	initiative, err := b.Initiatives.Polling(ctx, initiativeID)
	if err == nil {
		var member bool
		if member, err = b.Members.MayViewInitiatives(ctx, userID, initiative.HouseID); err == nil && !member {
			b.sendHTML(ctx, maxUserID, "Задавать вопросы могут подтверждённые жители дома.", nil)

			return
		}
	}
	if err == nil {
		_, err = b.Questions.AskQuestion(ctx, initiativeID, userID, text)
	}

	switch {
	case err == nil:
		b.done(ctx, maxUserID, d, fmt.Sprintf("✅ Вопрос отправлен инициатору:\n<blockquote>%s</blockquote>\n"+
			"Ответ придёт сюда.", esc(strings.TrimSpace(text))))
	case errors.Is(err, initiatives.ErrTextTooLong):
		b.prompt(ctx, maxUserID, dialogQuestion, initiativeID, fmt.Sprintf(
			"Вопрос длиннее %d символов. Сократите его и отправьте ещё раз.", initiatives.MaxQuestionLen))
	case errors.Is(err, initiatives.ErrOwnInitiative):
		b.sendHTML(ctx, maxUserID, "Это ваша инициатива: вопросы задают соседи, а вы отвечаете на них здесь.", nil)
	case errors.Is(err, initiatives.ErrTooManyQuestions):
		b.sendHTML(ctx, maxUserID, fmt.Sprintf("По этой инициативе можно задать не больше %d вопросов в сутки. "+
			"Попробуйте завтра.", initiatives.MaxQuestionsPerDay), nil)
	case errors.Is(err, initiatives.ErrQuestionsClosed), errors.Is(err, initiatives.ErrNoInitiatorToReach):
		b.sendHTML(ctx, maxUserID, "Инициатива завершена, вопросы по ней больше не принимаются.", nil)
	case errors.Is(err, initiatives.ErrNotFound):
		b.sendHTML(ctx, maxUserID, "Инициатива не найдена.", nil)
	default:
		b.Log.Error("bot: ask question", "err", err, "initiative", initiativeID)
		b.sendHTML(ctx, maxUserID, "Не получилось отправить вопрос, попробуйте ещё раз.", nil)
	}
}

func (b *Bot) answerQuestion(ctx context.Context, maxUserID int64, userID string, d dialog, text string) {
	questionID := d.ref
	_, err := b.Questions.AnswerQuestion(ctx, questionID, userID, text)
	switch {
	case err == nil:
		b.done(ctx, maxUserID, d, fmt.Sprintf("✅ Ответ отправлен соседу:\n<blockquote>%s</blockquote>",
			esc(strings.TrimSpace(text))))
	case errors.Is(err, initiatives.ErrTextTooLong):
		b.prompt(ctx, maxUserID, dialogAnswer, questionID, fmt.Sprintf(
			"Ответ длиннее %d символов. Сократите его и отправьте ещё раз.", initiatives.MaxAnswerLen))
	case errors.Is(err, initiatives.ErrQuestionAnswered):
		b.sendHTML(ctx, maxUserID, "На этот вопрос вы уже ответили.", nil)
	case errors.Is(err, initiatives.ErrNotInitiator):
		b.sendHTML(ctx, maxUserID, "Ответить на вопрос может только инициатор.", nil)
	case errors.Is(err, initiatives.ErrQuestionNotFound):
		b.sendHTML(ctx, maxUserID, "Вопрос не найден: возможно, сосед удалил аккаунт.", nil)
	default:
		b.Log.Error("bot: answer question", "err", err, "question", questionID)
		b.sendHTML(ctx, maxUserID, "Не получилось отправить ответ, попробуйте ещё раз.", nil)
	}
}

func cancelKeyboard() *model.Keyboard {
	kb := model.NewKeyboard()
	kb.AddRow().AddButton(model.Button{Type: model.ButtonCallback, Text: "Отмена", Payload: cancelPayload})

	return kb
}

// QuestionRelay executes the jobs of questions: it delivers a question to the
// initiator and an answer to the asker (notify.TypeQuestionAsked, TypeQuestionAnswered).
// A job without anyone to deliver to — a deleted account or question — is done.
type QuestionRelay struct {
	Messages    Messages
	Questions   Questions
	Initiatives InitiativeReader
	Houses      HouseReader
	Accounts    Accounts
	Me          Identity
}

// HandleAsked sends the question to the initiator with the button to answer it.
func (r *QuestionRelay) HandleAsked(ctx context.Context, payload json.RawMessage) error {
	q, initiative, ok, err := r.load(ctx, payload)
	if !ok || err != nil {
		return err
	}
	if initiative.InitiatorUserID == nil {
		return nil
	}
	to, ok, err := r.maxUserID(ctx, *initiative.InitiatorUserID)
	if !ok || err != nil {
		return err
	}
	house, err := r.Houses.House(ctx, initiative.HouseID)
	if err != nil {
		return fmt.Errorf("question house: %w", err)
	}

	text := fmt.Sprintf("<b>Сосед спрашивает про вашу инициативу «%s»</b>\n\n<blockquote>%s</blockquote>\n\n"+
		"Ответьте кнопкой ниже — ответ получит только тот, кто спросил. Кто это, мы не показываем.",
		esc(initiative.Title), esc(q.Text))
	kb := model.NewKeyboard()
	kb.AddRow().AddButton(model.Button{Type: model.ButtonCallback, Text: "Ответить", Payload: answerPayloadPrefix + ":" + q.ID})
	kb.AddRow().AddButton(appButton(r.Me, "Открыть приложение", house.InviteSlug))

	return r.send(ctx, to, text, kb)
}

// HandleAnswered sends the initiator's answer to the neighbour who asked.
func (r *QuestionRelay) HandleAnswered(ctx context.Context, payload json.RawMessage) error {
	q, initiative, ok, err := r.load(ctx, payload)
	if !ok || err != nil || q.AnsweredAt == nil {
		return err
	}
	to, ok, err := r.maxUserID(ctx, q.AskedByUserID)
	if !ok || err != nil {
		return err
	}
	house, err := r.Houses.House(ctx, initiative.HouseID)
	if err != nil {
		return fmt.Errorf("answer house: %w", err)
	}

	text := fmt.Sprintf("<b>Инициатор ответил на ваш вопрос про «%s»</b>\n\n<i>Ваш вопрос:</i>\n<blockquote>%s</blockquote>\n\n"+
		"<i>Ответ:</i>\n<blockquote>%s</blockquote>", esc(initiative.Title), esc(q.Text), esc(q.Answer))

	return r.send(ctx, to, text, openAppButton(r.Me, "Открыть приложение", house.InviteSlug))
}

// load reads the question and its initiative; ok is false when either is gone.
func (r *QuestionRelay) load(ctx context.Context, payload json.RawMessage) (initiatives.Question, initiatives.Initiative, bool, error) {
	var job struct {
		QuestionID string `json:"question_id"`
	}
	if err := json.Unmarshal(payload, &job); err != nil || job.QuestionID == "" {
		return initiatives.Question{}, initiatives.Initiative{}, false, fmt.Errorf("question payload: %s", payload)
	}
	q, err := r.Questions.Question(ctx, job.QuestionID)
	if errors.Is(err, initiatives.ErrQuestionNotFound) {
		return q, initiatives.Initiative{}, false, nil
	}
	if err != nil {
		return q, initiatives.Initiative{}, false, fmt.Errorf("question: %w", err)
	}
	initiative, err := r.Initiatives.Get(ctx, q.InitiativeID)
	if errors.Is(err, initiatives.ErrNotFound) {
		return q, initiative, false, nil
	}
	if err != nil {
		return q, initiative, false, fmt.Errorf("question initiative: %w", err)
	}

	return q, initiative, true, nil
}

func (r *QuestionRelay) maxUserID(ctx context.Context, userID string) (int64, bool, error) {
	id, err := r.Accounts.MaxUserID(ctx, userID)
	if errors.Is(err, access.ErrNotFound) {
		return 0, false, nil // the account is deleted
	}
	if err != nil {
		return 0, false, fmt.Errorf("max user id: %w", err)
	}

	return id, true, nil
}

func (r *QuestionRelay) send(ctx context.Context, maxUserID int64, text string, kb *model.Keyboard) error {
	msg := maxapi.NewMessage().SetUser(maxUserID).SetText(text).SetFormat(model.FormatHTML).AddKeyboard(kb)
	if _, err := r.Messages.Send(ctx, msg); err != nil {
		return fmt.Errorf("relay send: %w", err)
	}

	return nil
}

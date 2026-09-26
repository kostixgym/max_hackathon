package bot

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/initiatives"
)

const testQuestion = "33333333-3333-7333-8333-333333333333"

// fakeQuestions is the initiatives module with one question of «user-7» to the
// initiative of «user-42» (fakeUsers gives every caller the id «user-42»).
type fakeQuestions struct {
	mu       sync.Mutex
	asked    []string // texts
	answered []string
	askErr   error
	answer   string // the stored answer of testQuestion
}

func (f *fakeQuestions) AskQuestion(_ context.Context, initiativeID, userID, text string) (initiatives.Question, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.askErr != nil {
		return initiatives.Question{}, f.askErr
	}
	f.asked = append(f.asked, text)

	return initiatives.Question{ID: testQuestion, InitiativeID: initiativeID, AskedByUserID: userID, Text: text}, nil
}

func (f *fakeQuestions) AnswerQuestion(_ context.Context, questionID, _, text string) (initiatives.Question, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answered = append(f.answered, text)

	return initiatives.Question{ID: questionID, Answer: text}, nil
}

func (f *fakeQuestions) Question(_ context.Context, id string) (initiatives.Question, error) {
	if id != testQuestion {
		return initiatives.Question{}, initiatives.ErrQuestionNotFound
	}
	q := initiatives.Question{ID: id, InitiativeID: testInitiative, AskedByUserID: "user-7",
		Text: `Сколько стоит? <script>alert(1)</script>`}
	if f.answer != "" {
		at := testNow
		q.Answer, q.AnsweredAt = f.answer, &at
	}

	return q, nil
}

type fakeMembers struct{ member bool }

func (f fakeMembers) MayViewInitiatives(context.Context, string, string) (bool, error) {
	return f.member, nil
}

// fakeInitiatives reads the initiative of testInitiative led by initiator.
type fakeInitiatives struct {
	initiator string
	stage     string
	endsAt    time.Time
}

func (f fakeInitiatives) Get(_ context.Context, id string) (initiatives.Initiative, error) {
	if id != testInitiative {
		return initiatives.Initiative{}, initiatives.ErrNotFound
	}
	in := initiatives.Initiative{ID: id, HouseID: "house-1", Title: "Камеры & <b>домофон</b>", Stage: f.stage}
	if f.initiator != "" {
		in.InitiatorUserID = &f.initiator
	}
	if !f.endsAt.IsZero() {
		in.PollEndsAt = &f.endsAt
	}

	return in, nil
}

// fakeAccounts: user-N has MAX id 1000+N; a deleted account is missing.
type fakeAccounts struct{ deleted string }

func (f fakeAccounts) MaxUserID(_ context.Context, userID string) (int64, error) {
	switch userID {
	case f.deleted:
		return 0, access.ErrNotFound
	case "user-7":
		return 1007, nil
	case "user-42":
		return 1042, nil
	}

	return 0, access.ErrNotFound
}

func newQuestionBot(questions *fakeQuestions, member bool) (*Bot, *fakeMessages, *fakeAnswers) {
	b := newVoteBot(&fakeVotes{})
	msgs, answers := &fakeMessages{}, &fakeAnswers{}
	b.Messages, b.Answers = msgs, answers
	b.Initiatives, b.HousesByID = fakePolling{stage: "poll"}, fakeHouseReader{}
	b.Questions, b.Members = questions, fakeMembers{member: member}
	b.InitiativeReader = fakeInitiatives{initiator: "user-42", stage: "poll"}

	return b, msgs, answers
}

func textUpdate(maxUserID int64, text string) model.Update {
	return model.Update{
		UpdateType: model.UpdateMessageCreated,
		UserID:     maxUserID,
		ChatID:     7,
		Message: &model.MessageUpdate{
			Sender:    model.Sender{UserID: maxUserID},
			Recipient: model.Recipient{ChatID: 7, ChatType: model.ChatTypeDialog},
			Body:      model.MessageBody{Text: text},
		},
	}
}

func lastText(msgs *fakeMessages) string {
	if len(msgs.sent) == 0 {
		return ""
	}

	return msgs.sent[len(msgs.sent)-1].body.Text
}

// «Есть вопрос» → the bot asks for the text → the next text is the question.
func TestQuestionFlow(t *testing.T) {
	questions := &fakeQuestions{}
	b, msgs, answers := newQuestionBot(questions, true)
	ctx := context.Background()

	b.Handle(ctx, callbackUpdate("cb-q", "pv:"+testInitiative+":question", 42))
	if len(answers.calls) != 1 {
		t.Fatalf("the press must be answered: %v", answers.calls)
	}
	prompt := msgs.sent[0].body
	if !strings.Contains(prompt.Text, "Напишите вопрос инициатору") || !strings.Contains(prompt.Text, "Кто спросил, он не увидит") {
		t.Fatalf("prompt = %q", prompt.Text)
	}
	if btn := prompt.Attachments[0].Payload.Buttons[0][0]; btn.Payload != cancelPayload {
		t.Fatalf("the prompt must offer «Отмена»: %+v", btn)
	}

	b.Handle(ctx, textUpdate(42, "  Кто будет смотреть записи?  "))
	if len(questions.asked) != 1 || questions.asked[0] != "  Кто будет смотреть записи?  " {
		t.Fatalf("asked = %q", questions.asked)
	}
	// The prompt turns into the confirmation with the question and loses «Отмена».
	done, ok := msgs.edits["mid-1"]
	if !ok || !strings.Contains(done.Text, "Вопрос отправлен инициатору") ||
		!strings.Contains(done.Text, "<blockquote>Кто будет смотреть записи?</blockquote>") ||
		done.Attachments == nil || len(done.Attachments) != 0 {
		t.Fatalf("prompt after the question = %+v", done)
	}
	if len(msgs.sent) != 1 {
		t.Fatalf("%d messages, want the prompt only: the confirmation replaces it", len(msgs.sent))
	}

	// The dialog is over: the next text is an ordinary message.
	b.Handle(ctx, textUpdate(42, "спасибо"))
	if len(questions.asked) != 1 || !strings.Contains(lastText(msgs), "приложении") {
		t.Fatalf("a text after the question: asked = %q, reply = %q", questions.asked, lastText(msgs))
	}
}

func TestQuestionOnlyForMembers(t *testing.T) {
	questions := &fakeQuestions{}
	b, msgs, _ := newQuestionBot(questions, false)

	b.Handle(context.Background(), callbackUpdate("cb-q", "pv:"+testInitiative+":question", 42))
	b.Handle(context.Background(), textUpdate(42, "вопрос"))

	if len(questions.asked) != 0 || !strings.Contains(msgs.sent[0].body.Text, "подтверждённые жители") {
		t.Fatalf("a guest asked: asked = %q, reply = %q", questions.asked, msgs.sent[0].body.Text)
	}

	// The link is revoked while the user types: the text is checked again.
	b, msgs, _ = newQuestionBot(questions, true)
	b.Handle(context.Background(), callbackUpdate("cb-q", "pv:"+testInitiative+":question", 42))
	b.Members = fakeMembers{member: false}
	b.Handle(context.Background(), textUpdate(42, "вопрос"))
	if len(questions.asked) != 0 || !strings.Contains(lastText(msgs), "подтверждённые жители") {
		t.Fatalf("revoked member asked: asked = %q, reply = %q", questions.asked, lastText(msgs))
	}
}

func TestQuestionAfterTheInitiativeIsOver(t *testing.T) {
	questions := &fakeQuestions{}
	b, msgs, _ := newQuestionBot(questions, true)
	b.Initiatives = fakePolling{stage: "canceled"}

	b.Handle(context.Background(), callbackUpdate("cb-q", "pv:"+testInitiative+":question", 42))
	b.Handle(context.Background(), textUpdate(42, "вопрос"))

	if len(questions.asked) != 0 || !strings.Contains(msgs.sent[0].body.Text, "Инициатива завершена") {
		t.Fatalf("asked = %q, reply = %q", questions.asked, msgs.sent[0].body.Text)
	}
}

// A pending question ends by a command, by «Отмена» or by time: a later message must
// not reach the initiator by accident.
func TestQuestionDialogEnds(t *testing.T) {
	ctx := context.Background()

	t.Run("command", func(t *testing.T) {
		questions := &fakeQuestions{}
		b, _, _ := newQuestionBot(questions, true)
		b.Handle(ctx, callbackUpdate("cb-q", "pv:"+testInitiative+":question", 42))
		b.Handle(ctx, textUpdate(42, "/start"))
		b.Handle(ctx, textUpdate(42, "текст после команды"))
		if len(questions.asked) != 0 {
			t.Fatalf("asked = %q", questions.asked)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		questions := &fakeQuestions{}
		b, _, answers := newQuestionBot(questions, true)
		b.Handle(ctx, callbackUpdate("cb-q", "pv:"+testInitiative+":question", 42))
		b.Handle(ctx, callbackUpdate("cb-x", cancelPayload, 42))
		b.Handle(ctx, textUpdate(42, "текст после отмены"))
		if len(questions.asked) != 0 {
			t.Fatalf("asked = %q", questions.asked)
		}
		// The prompt is replaced and loses its button: an empty list, not null.
		edit := answers.messages[len(answers.messages)-1]
		if edit == nil || edit.Text != "Отменено." || edit.Attachments == nil || len(edit.Attachments) != 0 {
			t.Fatalf("prompt after cancel = %+v", edit)
		}
	})

	t.Run("time", func(t *testing.T) {
		questions := &fakeQuestions{}
		b, _, _ := newQuestionBot(questions, true)
		now := testNow
		b.Now = func() time.Time { return now }
		b.Handle(ctx, callbackUpdate("cb-q", "pv:"+testInitiative+":question", 42))
		now = now.Add(dialogTTL)
		b.Handle(ctx, textUpdate(42, "текст через 15 минут"))
		if len(questions.asked) != 0 {
			t.Fatalf("asked = %q", questions.asked)
		}
	})

	t.Run("another user", func(t *testing.T) {
		questions := &fakeQuestions{}
		b, _, _ := newQuestionBot(questions, true)
		b.Handle(ctx, callbackUpdate("cb-q", "pv:"+testInitiative+":question", 42))
		b.Handle(ctx, textUpdate(43, "текст соседа"))
		if len(questions.asked) != 0 {
			t.Fatalf("asked = %q", questions.asked)
		}
	})
}

func TestQuestionWaitsForText(t *testing.T) {
	questions := &fakeQuestions{}
	b, msgs, _ := newQuestionBot(questions, true)
	ctx := context.Background()

	b.Handle(ctx, callbackUpdate("cb-q", "pv:"+testInitiative+":question", 42))
	b.Handle(ctx, textUpdate(42, "")) // a sticker or a photo
	if !strings.Contains(lastText(msgs), "Пришлите текст") {
		t.Fatalf("reply to a sticker = %q", lastText(msgs))
	}

	questions.askErr = initiatives.ErrTextTooLong
	b.Handle(ctx, textUpdate(42, "длинный вопрос"))
	if !strings.Contains(lastText(msgs), "длиннее 1000 символов") {
		t.Fatalf("reply to a long question = %q", lastText(msgs))
	}

	// Both keep the dialog: the shortened question goes through.
	questions.askErr = nil
	b.Handle(ctx, textUpdate(42, "короткий вопрос"))
	if len(questions.asked) != 1 || questions.asked[0] != "короткий вопрос" {
		t.Fatalf("asked = %q", questions.asked)
	}
}

func TestAnswerFlow(t *testing.T) {
	questions := &fakeQuestions{}
	b, msgs, _ := newQuestionBot(questions, true)
	ctx := context.Background()

	// fakeUsers makes the presser «user-42», the initiator of fakeInitiatives.
	b.Handle(ctx, callbackUpdate("cb-a", answerPayloadPrefix+":"+testQuestion, 42))
	if !strings.Contains(lastText(msgs), "Напишите ответ") {
		t.Fatalf("prompt = %q", lastText(msgs))
	}
	if !strings.Contains(lastText(msgs), "&lt;script&gt;") {
		t.Fatalf("the prompt must quote the question, escaped: %q", lastText(msgs))
	}
	b.Handle(ctx, textUpdate(42, "Записи смотрит только УК"))
	if len(questions.answered) != 1 || questions.answered[0] != "Записи смотрит только УК" {
		t.Fatalf("answered = %q", questions.answered)
	}
	if done := msgs.edits["mid-1"]; !strings.Contains(done.Text, "Ответ отправлен соседу") {
		t.Fatalf("prompt after the answer = %+v", done)
	}
}

// The initiator answers questions, they do not ask their own initiative: an old
// message with the button gets an explanation instead of a prompt.
func TestInitiatorDoesNotAskThemselves(t *testing.T) {
	questions := &fakeQuestions{}
	b, msgs, _ := newQuestionBot(questions, true)
	b.Initiatives = fakePolling{stage: "poll", initiator: "user-42"} // fakeUsers makes every presser «user-42»

	b.Handle(context.Background(), callbackUpdate("cb-q", "pv:"+testInitiative+":question", 42))
	b.Handle(context.Background(), textUpdate(42, "вопрос самому себе"))

	if len(questions.asked) != 0 || !strings.Contains(msgs.sent[0].body.Text, "Это ваша инициатива") {
		t.Fatalf("asked = %q, reply = %q", questions.asked, msgs.sent[0].body.Text)
	}
}

func pollLinkUpdate(maxUserID int64, initiativeID string) model.Update {
	return model.Update{UpdateType: model.UpdateBotStarted, UserID: maxUserID, ChatID: 7,
		Payload: pollStartPrefix + initiativeID, User: &model.User{UserID: maxUserID}}
}

// The initiator shares the poll link; the bot sends the poll to a member of the house
// who opens it, and only the title to anyone else (решение 19).
func TestPollLink(t *testing.T) {
	ctx := context.Background()

	b, msgs, _ := newQuestionBot(&fakeQuestions{}, true)
	b.Progress = fakeProgress{forCenti: 6350}
	b.Handle(ctx, pollLinkUpdate(43, testInitiative))
	if len(msgs.sent) != 1 || msgs.sent[0].to != 43 {
		t.Fatalf("sent = %+v, want the poll to the neighbour 43", msgs.sent)
	}
	body := msgs.sent[0].body
	if !strings.Contains(body.Text, "Опрос соседей: «Камеры»") || !strings.Contains(body.Text, "63,50 м²") {
		t.Fatalf("text = %s", body.Text)
	}
	if buttons := body.Attachments[0].Payload.Buttons; buttons[1][0].Payload != "pv:"+testInitiative+":question" {
		t.Fatalf("a neighbour asks questions: %+v", buttons)
	}

	b, msgs, _ = newQuestionBot(&fakeQuestions{}, false)
	b.Handle(ctx, pollLinkUpdate(43, testInitiative))
	if text := msgs.sent[0].body.Text; !strings.Contains(text, "Соседи обсуждают «Камеры»") ||
		!strings.Contains(text, "подтверждённые жители") || strings.Contains(text, "Поддержали") {
		t.Fatalf("not a member: %s", text)
	}

	// A draft or an unknown poll: the ordinary greeting, nothing about the initiative.
	for name, polling := range map[string]fakePolling{
		"draft":   {stage: "draft"},
		"unknown": {err: initiatives.ErrNotFound},
	} {
		b, msgs, _ = newQuestionBot(&fakeQuestions{}, true)
		b.Initiatives = polling
		b.Handle(ctx, pollLinkUpdate(43, testInitiative))
		if len(msgs.sent) != 1 || strings.Contains(msgs.sent[0].body.Text, "Камеры") ||
			!strings.Contains(msgs.sent[0].body.Text, "Здравствуйте") {
			t.Fatalf("%s: %+v", name, msgs.sent)
		}
	}
}

func TestAnswerOnlyByInitiatorAndOnce(t *testing.T) {
	ctx := context.Background()

	questions := &fakeQuestions{}
	b, msgs, _ := newQuestionBot(questions, true)
	b.InitiativeReader = fakeInitiatives{initiator: "user-7", stage: "poll"}
	b.Handle(ctx, callbackUpdate("cb-a", answerPayloadPrefix+":"+testQuestion, 42))
	b.Handle(ctx, textUpdate(42, "чужой ответ"))
	if len(questions.answered) != 0 || !strings.Contains(msgs.sent[0].body.Text, "только инициатор") {
		t.Fatalf("not the initiator: answered = %q, reply = %q", questions.answered, msgs.sent[0].body.Text)
	}

	questions = &fakeQuestions{answer: "уже ответил"}
	b, msgs, _ = newQuestionBot(questions, true)
	b.Handle(ctx, callbackUpdate("cb-a", answerPayloadPrefix+":"+testQuestion, 42))
	if !strings.Contains(msgs.sent[0].body.Text, "уже ответили") {
		t.Fatalf("answered twice: reply = %q", msgs.sent[0].body.Text)
	}
}

func relayJob(t *testing.T, questionID string) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"question_id": questionID})
	if err != nil {
		t.Fatal(err)
	}

	return payload
}

func newRelay(questions *fakeQuestions, accounts fakeAccounts) (*QuestionRelay, *fakeMessages) {
	msgs := &fakeMessages{}

	return &QuestionRelay{
		Messages: msgs, Questions: questions, Initiatives: fakeInitiatives{initiator: "user-42", stage: "poll"},
		Houses: fakeHouseReader{}, Accounts: accounts, Me: Identity{UserID: 555, Username: "dom_test_bot"},
	}, msgs
}

// The question reaches the initiator without the name of the asker, and nobody's text
// can inject markup into a message.
func TestRelayQuestionToInitiator(t *testing.T) {
	relay, msgs := newRelay(&fakeQuestions{}, fakeAccounts{})
	if err := relay.HandleAsked(context.Background(), relayJob(t, testQuestion)); err != nil {
		t.Fatal(err)
	}
	if len(msgs.sent) != 1 || msgs.sent[0].to != 1042 {
		t.Fatalf("sent = %+v, want one message to the initiator 1042", msgs.sent)
	}
	body := msgs.sent[0].body
	if !strings.Contains(body.Text, "Сосед спрашивает про вашу инициативу") {
		t.Fatalf("who asks whom must be clear: %s", body.Text)
	}
	if body.Format != model.FormatHTML || !strings.Contains(body.Text, "&lt;script&gt;") ||
		strings.Contains(body.Text, "<script>") || !strings.Contains(body.Text, "Камеры &amp; &lt;b&gt;домофон&lt;/b&gt;") {
		t.Fatalf("text = %q, want the user texts escaped", body.Text)
	}
	if strings.Contains(body.Text, "user-7") || strings.Contains(body.Text, "1007") {
		t.Fatalf("the asker is named: %q", body.Text)
	}
	if btn := body.Attachments[0].Payload.Buttons[0][0]; btn.Payload != answerPayloadPrefix+":"+testQuestion {
		t.Fatalf("the answer button = %+v", btn)
	}
}

func TestRelayAnswerToAsker(t *testing.T) {
	relay, msgs := newRelay(&fakeQuestions{answer: "Только <УК>"}, fakeAccounts{})
	if err := relay.HandleAnswered(context.Background(), relayJob(t, testQuestion)); err != nil {
		t.Fatal(err)
	}
	if len(msgs.sent) != 1 || msgs.sent[0].to != 1007 || !strings.Contains(msgs.sent[0].body.Text, "Только &lt;УК&gt;") ||
		!strings.Contains(msgs.sent[0].body.Text, "Инициатор ответил на ваш вопрос") {
		t.Fatalf("sent = %+v, want the escaped answer to the asker 1007", msgs.sent)
	}
}

// A job with nobody to deliver to is done, not retried forever.
func TestRelaySkipsWhatIsGone(t *testing.T) {
	ctx := context.Background()

	relay, msgs := newRelay(&fakeQuestions{}, fakeAccounts{})
	if err := relay.HandleAsked(ctx, relayJob(t, "44444444-4444-7444-8444-444444444444")); err != nil || len(msgs.sent) != 0 {
		t.Fatalf("deleted question: err = %v, sent = %d", err, len(msgs.sent))
	}

	relay, msgs = newRelay(&fakeQuestions{}, fakeAccounts{deleted: "user-42"})
	if err := relay.HandleAsked(ctx, relayJob(t, testQuestion)); err != nil || len(msgs.sent) != 0 {
		t.Fatalf("deleted initiator: err = %v, sent = %d", err, len(msgs.sent))
	}

	relay, msgs = newRelay(&fakeQuestions{}, fakeAccounts{}) // not answered yet
	if err := relay.HandleAnswered(ctx, relayJob(t, testQuestion)); err != nil || len(msgs.sent) != 0 {
		t.Fatalf("no answer: err = %v, sent = %d", err, len(msgs.sent))
	}

	if err := relay.HandleAsked(ctx, json.RawMessage(`{}`)); err == nil {
		t.Fatal("a job without a question id must fail loudly")
	}
}

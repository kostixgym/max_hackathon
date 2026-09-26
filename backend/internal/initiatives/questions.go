package initiatives

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"maxhackathon/backend/internal/notify"
)

// Questions of neighbours to the initiator (docs/04, решения 30 и 78): «Есть вопрос»
// in the poll message. The question goes to the initiator without the name of the
// asker, the answer comes back to the asker. Who may ask (a verified member of the
// house) is checked by the adapter, like every access rule.

// Question is a neighbour's question with the initiator's answer.
type Question struct {
	ID            string
	InitiativeID  string
	AskedByUserID string
	Text          string
	Answer        string // empty until answered
	AnsweredAt    *time.Time
	CreatedAt     time.Time
}

// Limits of questions.
const (
	MaxQuestionLen = 1000
	MaxAnswerLen   = 2000
	// MaxQuestionsPerDay: one user asks one initiative at most this many questions in
	// 24 hours. Each question is a bot message to the initiator: the limit keeps a
	// neighbour from flooding them.
	MaxQuestionsPerDay = 5
)

// Errors of questions.
var (
	ErrEmptyText          = errors.New("the text is empty")
	ErrTextTooLong        = errors.New("the text is too long")
	ErrTooManyQuestions   = errors.New("too many questions to the initiative in the last 24 hours")
	ErrQuestionsClosed    = errors.New("the initiative takes no questions at this stage")
	ErrQuestionNotFound   = errors.New("question not found")
	ErrQuestionAnswered   = errors.New("the question is already answered")
	ErrNoInitiatorToReach = errors.New("the initiative has no initiator to answer")
	// ErrOwnInitiative: the initiator answers questions, they do not ask themselves.
	ErrOwnInitiative = errors.New("the initiator does not ask questions to their own initiative")
)

// takesQuestions: a question makes sense while the initiative goes on. A draft is
// not seen by the neighbours (решение 76), a finished one has nobody to answer.
func takesQuestions(stage string) bool {
	return stage == StagePoll || stage == StageDemand || stage == StageMeeting
}

// AskQuestion stores the question and queues it to the initiator — in one
// transaction, so an accepted question is always delivered.
func (s *Service) AskQuestion(ctx context.Context, initiativeID, userID, text string) (Question, error) {
	text, err := cleanText(text, MaxQuestionLen)
	if err != nil {
		return Question{}, err
	}
	in, err := s.Get(ctx, initiativeID)
	if err != nil {
		return Question{}, err
	}
	if !takesQuestions(in.Stage) {
		return Question{}, fmt.Errorf("%w: %s", ErrQuestionsClosed, in.Stage)
	}
	if in.InitiatorUserID == nil {
		return Question{}, ErrNoInitiatorToReach
	}
	if *in.InitiatorUserID == userID {
		return Question{}, ErrOwnInitiative
	}

	q := Question{InitiativeID: in.ID, AskedByUserID: userID, Text: text}
	err = s.tm.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var recent int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM initiative_questions
			WHERE initiative_id = $1::uuid AND asked_by_user_id = $2::uuid
			  AND created_at > now() - interval '24 hours'`, in.ID, userID).Scan(&recent); err != nil {
			return fmt.Errorf("count recent questions: %w", err)
		}
		if recent >= MaxQuestionsPerDay {
			return ErrTooManyQuestions
		}

		if err := tx.QueryRow(ctx, `
			INSERT INTO initiative_questions (initiative_id, asked_by_user_id, text)
			VALUES ($1::uuid, $2::uuid, $3)
			RETURNING id::text, created_at`, in.ID, userID, text).Scan(&q.ID, &q.CreatedAt); err != nil {
			return fmt.Errorf("insert question: %w", err)
		}

		runAt, err := s.relayTime(ctx, in.HouseID)
		if err != nil {
			return err
		}

		return s.queue.EnqueueTx(ctx, tx, notify.Job{
			Type:     notify.TypeQuestionAsked,
			DedupKey: notify.TypeQuestionAsked + ":" + q.ID,
			Payload:  map[string]any{"question_id": q.ID},
			RunAt:    runAt,
		})
	})
	if err != nil {
		return Question{}, err
	}

	return q, nil
}

// AnswerQuestion stores the initiator's answer and queues it to the asker. A
// question is answered once.
func (s *Service) AnswerQuestion(ctx context.Context, questionID, userID, text string) (Question, error) {
	text, err := cleanText(text, MaxAnswerLen)
	if err != nil {
		return Question{}, err
	}
	q, err := s.Question(ctx, questionID)
	if err != nil {
		return Question{}, err
	}
	in, err := s.Get(ctx, q.InitiativeID)
	if errors.Is(err, ErrNotFound) {
		return Question{}, ErrQuestionNotFound
	}
	if err != nil {
		return Question{}, err
	}
	if in.InitiatorUserID == nil || *in.InitiatorUserID != userID {
		return Question{}, ErrNotInitiator
	}
	if q.AnsweredAt != nil {
		return Question{}, ErrQuestionAnswered
	}

	err = s.tm.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var answeredAt time.Time
		err := tx.QueryRow(ctx, `
			UPDATE initiative_questions
			SET answer = $2, answered_by_user_id = $3::uuid, answered_at = now()
			WHERE id = $1::uuid AND answered_at IS NULL
			RETURNING answered_at`, q.ID, text, userID).Scan(&answeredAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrQuestionAnswered // answered a moment ago in another request
		}
		if err != nil {
			return fmt.Errorf("answer question: %w", err)
		}
		q.Answer, q.AnsweredAt = text, &answeredAt

		runAt, err := s.relayTime(ctx, in.HouseID)
		if err != nil {
			return err
		}

		return s.queue.EnqueueTx(ctx, tx, notify.Job{
			Type:     notify.TypeQuestionAnswered,
			DedupKey: notify.TypeQuestionAnswered + ":" + q.ID,
			Payload:  map[string]any{"question_id": q.ID},
			RunAt:    runAt,
		})
	})
	if err != nil {
		return Question{}, err
	}

	return q, nil
}

// Question returns a question by id.
func (s *Service) Question(ctx context.Context, id string) (Question, error) {
	var q Question
	if !validID(id) {
		return q, ErrQuestionNotFound
	}
	var answer *string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, initiative_id::text, asked_by_user_id::text, text, answer, answered_at, created_at
		FROM initiative_questions
		WHERE id = $1::uuid`, id,
	).Scan(&q.ID, &q.InitiativeID, &q.AskedByUserID, &q.Text, &answer, &q.AnsweredAt, &q.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return q, ErrQuestionNotFound
	}
	if err != nil {
		return q, fmt.Errorf("question: %w", err)
	}
	if answer != nil {
		q.Answer = *answer
	}

	return q, nil
}

// relayTime: a question or an answer written at night reaches the other side in the
// morning (решение 29). Zero means now.
func (s *Service) relayTime(ctx context.Context, houseID string) (time.Time, error) {
	house, err := s.registry.House(ctx, houseID)
	if err != nil {
		return time.Time{}, err
	}
	if until, quiet := notify.QuietHoursEnd(time.Now(), house.Location()); quiet {
		return until, nil
	}

	return time.Time{}, nil
}

// cleanText trims the text and checks its length in characters.
func cleanText(text string, maxLen int) (string, error) {
	text = strings.TrimSpace(text)
	switch {
	case text == "":
		return "", ErrEmptyText
	case utf8.RuneCountInString(text) > maxLen:
		return "", ErrTextTooLong
	}

	return text, nil
}

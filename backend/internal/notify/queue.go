// Package notify is the «Уведомления» module: jobs in PostgreSQL and bot polling
// markers. The worker of this module does everything that must not block a request:
// poll invitations, reminders and other mailings (docs/04, module 8).
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Job types.
const (
	// TypePollInvite sends the support-poll message with voting buttons
	// to one verified owner.
	TypePollInvite = "poll_invite"
	// TypePollReminder sends one mid-term reminder; the handler skips owners who voted.
	TypePollReminder = "poll_reminder"
	// TypePollFinished sends the result of the poll to the initiator when its term ends.
	TypePollFinished = "poll_finished"
	// TypeQuestionAsked relays a neighbour's question to the initiator.
	TypeQuestionAsked = "question_asked"
	// TypeQuestionAnswered relays the initiator's answer to the neighbour who asked.
	TypeQuestionAnswered = "question_answered"

	// Sprint to 30.09 (docs/plan-do-30-09.md): the data change enqueues the job in
	// its own transaction, the bot (Костя) sends the messages.
	TypeDemandDelivered  = "demand_delivered"  // требование передано в УК → сотрудникам УК
	TypeMeetingCreated   = "meeting_created"   // собрание создано → собственникам дома
	TypeMeetingFinalized = "meeting_finalized" // итог зафиксирован → инициатору и собственникам
)

// Job is one unit of background work.
type Job struct {
	Type     string
	Payload  map[string]any
	DedupKey string    // empty means no deduplication
	RunAt    time.Time // zero means now
}

// Store reads and writes the module tables: jobs and bot_markers.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a notify store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// EnqueueTx adds jobs inside the caller's transaction: a poll starts and its
// invitations become visible atomically (docs/04, «загрузка реестра» pattern:
// data change and jobs in one transaction).
func (s *Store) EnqueueTx(ctx context.Context, tx pgx.Tx, jobs ...Job) error {
	for i, j := range jobs {
		payload := j.Payload
		if payload == nil {
			payload = map[string]any{} // NULL jsonb would break the object check
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("job %d payload: %w", i, err)
		}

		var dedup any
		if j.DedupKey != "" {
			dedup = j.DedupKey
		}

		// run_at defaults to the database clock: a client clock even slightly ahead
		// would postpone the job into the future.
		_, err = tx.Exec(ctx, `
			INSERT INTO jobs (type, payload, dedup_key, run_at)
			VALUES ($1, $2, $3, COALESCE($4::timestamptz, now()))
			ON CONFLICT (dedup_key) DO NOTHING`, j.Type, data, dedup, runAtOrNull(j.RunAt))
		if err != nil {
			return fmt.Errorf("enqueue job %d: %w", i, err)
		}
	}

	return nil
}

// Enqueue adds jobs outside a transaction.
func (s *Store) Enqueue(ctx context.Context, jobs ...Job) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return s.EnqueueTx(ctx, tx, jobs...)
	})
}

// ClaimedJob is a job taken by a worker.
type ClaimedJob struct {
	ID          string
	Type        string
	Payload     json.RawMessage
	Attempts    int
	MaxAttempts int
}

// Claim takes up to limit queued jobs whose time has come. FOR UPDATE SKIP LOCKED
// lets several workers work on one queue without taking the same job twice.
//
// The picked rows are a MATERIALIZED CTE: in «UPDATE … WHERE id IN (SELECT … LIMIT n
// FOR UPDATE SKIP LOCKED)» PostgreSQL may run the subquery more than once, each run
// skipping the rows the previous one locked, and claim more than limit jobs.
func (s *Store) Claim(ctx context.Context, limit int) ([]ClaimedJob, error) {
	rows, err := s.pool.Query(ctx, `
		WITH picked AS MATERIALIZED (
			SELECT id FROM jobs
			WHERE status = 'queued' AND run_at <= now()
			ORDER BY run_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED)
		UPDATE jobs j SET status = 'running', attempts = j.attempts + 1, updated_at = now()
		FROM picked
		WHERE j.id = picked.id
		RETURNING j.id::text, j.type, j.payload, j.attempts, j.max_attempts`, limit)
	if err != nil {
		return nil, fmt.Errorf("claim jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]ClaimedJob, 0, limit)
	for rows.Next() {
		var j ClaimedJob
		if err := rows.Scan(&j.ID, &j.Type, &j.Payload, &j.Attempts, &j.MaxAttempts); err != nil {
			return nil, fmt.Errorf("scan claimed job: %w", err)
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim jobs: %w", err)
	}

	return jobs, nil
}

// Complete marks a job done.
func (s *Store) Complete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE jobs SET status = 'done', updated_at = now() WHERE id = $1::uuid`, id)
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}

	return nil
}

// Retry puts a failed job back with a pause, or fails it for good when the attempt
// budget is spent.
func (s *Store) Retry(ctx context.Context, id string, jobErr error, pause time.Duration) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'queued' END,
		    run_at = CASE WHEN attempts >= max_attempts THEN run_at ELSE now() + $2 END,
		    last_error = left($3, 2000), updated_at = now()
		WHERE id = $1::uuid`, id, pause, jobErr.Error())
	if err != nil {
		return fmt.Errorf("retry job: %w", err)
	}

	return nil
}

// ResetStale requeues jobs stuck in 'running' after a worker crash; ten minutes is
// longer than any healthy handler, so a job of a live worker is never taken away.
// A job that has spent its attempts fails: a job that kills the worker every time
// must not loop forever.
func (s *Store) ResetStale(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'queued' END,
		    last_error = CASE WHEN attempts >= max_attempts THEN 'stuck in running' ELSE last_error END,
		    updated_at = now()
		WHERE status = 'running' AND updated_at < now() - interval '10 minutes'`)
	if err != nil {
		return 0, fmt.Errorf("reset stale jobs: %w", err)
	}

	return tag.RowsAffected(), nil
}

// Purge deletes jobs finished more than 30 days ago. Their payload holds the MAX id
// of the addressee, and personal data is kept no longer than needed (docs/04,
// «Безопасность и персональные данные»). By then any poll is long over, so the
// dedup keys of the deleted jobs are no longer needed either.
func (s *Store) Purge(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM jobs
		WHERE status IN ('done', 'failed') AND updated_at < now() - interval '30 days'`)
	if err != nil {
		return 0, fmt.Errorf("purge jobs: %w", err)
	}

	return tag.RowsAffected(), nil
}

// runAtOrNull converts a zero time to SQL NULL.
func runAtOrNull(t time.Time) any {
	if t.IsZero() {
		return nil
	}

	return t
}

// ErrNoHandler means that nobody registered a handler for the job type.
var ErrNoHandler = errors.New("no handler for job type")

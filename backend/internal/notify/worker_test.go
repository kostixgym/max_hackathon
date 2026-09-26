package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// fakeQueue hands out its jobs once and records what the worker did with them.
type fakeQueue struct {
	mu         sync.Mutex
	jobs       []ClaimedJob
	completed  []string
	retried    map[string]time.Duration
	staleCalls int
	purgeCalls int
}

func (q *fakeQueue) Claim(context.Context, int) ([]ClaimedJob, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	jobs := q.jobs
	q.jobs = nil

	return jobs, nil
}

func (q *fakeQueue) Complete(_ context.Context, id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.completed = append(q.completed, id)

	return nil
}

func (q *fakeQueue) Retry(ctx context.Context, id string, _ error, pause time.Duration) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err() // a requeue with a dead context would never reach the database
	}
	q.retried[id] = pause

	return nil
}

func (q *fakeQueue) ResetStale(context.Context) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.staleCalls++

	return 0, nil
}

func (q *fakeQueue) Purge(context.Context) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.purgeCalls++

	return 0, nil
}

func newWorker(q *fakeQueue, handlers map[string]JobHandler) *Worker {
	return &Worker{
		Queue: q, Handlers: handlers, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		PollInterval: 5 * time.Millisecond, Maintenance: 5 * time.Millisecond,
	}
}

func TestWorkerCompletesAndRetries(t *testing.T) {
	q := &fakeQueue{retried: map[string]time.Duration{}, jobs: []ClaimedJob{
		{ID: "ok", Type: "t", Attempts: 1},
		{ID: "bad", Type: "t", Attempts: 2},
		{ID: "unknown", Type: "nobody-handles-it", Attempts: 1},
	}}
	calls := 0
	w := newWorker(q, map[string]JobHandler{"t": func(context.Context, json.RawMessage) error {
		calls++
		if calls == 2 {
			return errors.New("MAX is down")
		}

		return nil
	}})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.completed) != 1 || q.completed[0] != "ok" {
		t.Fatalf("completed = %v, want [ok]", q.completed)
	}
	if q.retried["bad"] != 5*time.Minute || q.retried["unknown"] != time.Minute {
		t.Fatalf("retried = %v, want bad in 5m (2nd attempt), unknown in 1m", q.retried)
	}
}

// A job interrupted by the service stop goes back to the queue at once instead of
// staying 'running' until the stale check of a later start.
func TestWorkerRequeuesJobOnStop(t *testing.T) {
	q := &fakeQueue{retried: map[string]time.Duration{}, jobs: []ClaimedJob{{ID: "slow", Type: "t", Attempts: 1}}}
	ctx, cancel := context.WithCancel(context.Background())
	w := newWorker(q, map[string]JobHandler{"t": func(ctx context.Context, _ json.RawMessage) error {
		cancel() // SIGTERM arrives while the message is being sent
		<-ctx.Done()

		return ctx.Err()
	}})
	w.Run(ctx)

	q.mu.Lock()
	defer q.mu.Unlock()
	if pause, ok := q.retried["slow"]; !ok || pause != 0 {
		t.Fatalf("retried = %v, want the job requeued with no pause", q.retried)
	}
	if len(q.completed) != 0 {
		t.Fatalf("completed = %v, want none", q.completed)
	}
}

// Stuck jobs are requeued and old jobs purged while the worker runs, not only on its start.
func TestWorkerMaintainsQueuePeriodically(t *testing.T) {
	q := &fakeQueue{retried: map[string]time.Duration{}}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	newWorker(q, nil).Run(ctx)

	q.mu.Lock()
	defer q.mu.Unlock()
	if q.staleCalls < 2 || q.purgeCalls < 2 {
		t.Fatalf("stale checks = %d, purges = %d, want several of each during the run", q.staleCalls, q.purgeCalls)
	}
}

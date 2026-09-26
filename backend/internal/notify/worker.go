package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// Queue is the part of the store the worker needs.
type Queue interface {
	Claim(ctx context.Context, limit int) ([]ClaimedJob, error)
	Complete(ctx context.Context, id string) error
	Retry(ctx context.Context, id string, jobErr error, pause time.Duration) error
	ResetStale(ctx context.Context) (int64, error)
	Purge(ctx context.Context) (int64, error)
}

// JobHandler executes jobs of one type. A returned error sends the job to a retry.
type JobHandler func(ctx context.Context, payload json.RawMessage) error

// Worker runs queued jobs in the background. Stage 1 runs it in the same process
// as the API (docs/03, вкладка 5D, этап 1); on growth it becomes a separate
// worker process reading the same tables.
type Worker struct {
	Queue    Queue
	Handlers map[string]JobHandler
	Log      *slog.Logger

	Batch        int           // jobs per claim, default 10
	PollInterval time.Duration // pause between empty claims, default 5s
	// Maintenance is how often jobs stuck in 'running' after a crash are requeued and
	// old finished jobs are purged, default 1 minute. Doing it only on start is not
	// enough: a container restarted right after a crash would leave its jobs stuck.
	Maintenance time.Duration
}

// Run works until ctx is cancelled. A panic in a handler sends the job to a retry
// but does not stop the worker.
func (w *Worker) Run(ctx context.Context) {
	if w.Batch <= 0 {
		w.Batch = 10
	}
	if w.PollInterval <= 0 {
		w.PollInterval = 5 * time.Second
	}
	if w.Maintenance <= 0 {
		w.Maintenance = time.Minute
	}

	w.Log.Info("notify: worker started")
	defer w.Log.Info("notify: worker stopped")

	var lastMaintenance time.Time
	for ctx.Err() == nil {
		if time.Since(lastMaintenance) >= w.Maintenance {
			w.maintain(ctx)
			lastMaintenance = time.Now()
		}

		jobs, err := w.Queue.Claim(ctx, w.Batch)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			w.Log.Warn("notify: claim jobs", "err", err)
			if !sleep(ctx, w.PollInterval) {
				return
			}

			continue
		}

		for _, j := range jobs {
			w.handle(ctx, j)
		}

		if len(jobs) == 0 && !sleep(ctx, w.PollInterval) {
			return
		}
	}
}

// maintain requeues jobs stuck after a crash and purges old finished jobs.
func (w *Worker) maintain(ctx context.Context) {
	if n, err := w.Queue.ResetStale(ctx); err != nil {
		if ctx.Err() == nil {
			w.Log.Warn("notify: reset stale jobs", "err", err)
		}
	} else if n > 0 {
		w.Log.Info("notify: requeued jobs stuck after a crash", "count", n)
	}

	if n, err := w.Queue.Purge(ctx); err != nil {
		if ctx.Err() == nil {
			w.Log.Warn("notify: purge old jobs", "err", err)
		}
	} else if n > 0 {
		w.Log.Info("notify: purged old jobs", "count", n)
	}
}

func (w *Worker) handle(ctx context.Context, j ClaimedJob) {
	err := w.runHandler(ctx, j)

	switch {
	case err == nil:
		if err := w.Queue.Complete(ctx, j.ID); err != nil {
			w.Log.Error("notify: complete job", "job_id", j.ID, "err", err)
		}
	case ctx.Err() != nil:
		// The service is stopping in the middle of the job: return it to the queue
		// right away. Left in 'running', it would wait for the stale check of the
		// next start. The request context is gone, so a short detached one is used.
		w.Log.Warn("notify: stopping during job, requeued", "job_id", j.ID, "type", j.Type)
		requeueCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := w.Queue.Retry(requeueCtx, j.ID, err, 0); err != nil {
			w.Log.Error("notify: requeue job on stop", "job_id", j.ID, "err", err)
		}
	default:
		pause := Backoff(j.Attempts)
		w.Log.Warn("notify: job failed, retrying", "job_id", j.ID, "type", j.Type,
			"attempt", j.Attempts, "in", pause, "err", err)
		if err := w.Queue.Retry(ctx, j.ID, err, pause); err != nil {
			w.Log.Error("notify: retry job", "job_id", j.ID, "err", err)
		}
	}
}

func (w *Worker) runHandler(ctx context.Context, j ClaimedJob) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("panic: %v", v)
		}
	}()

	handler, ok := w.Handlers[j.Type]
	if !ok {
		// Retried like any error: during a rolling update an old worker may take a
		// job type that only the new version knows.
		return fmt.Errorf("%w %q", ErrNoHandler, j.Type)
	}

	return handler(ctx, j.Payload)
}

// Backoff grows from 1 minute to 30 minutes (docs/03, вкладка 5C).
func Backoff(attempts int) time.Duration {
	switch {
	case attempts <= 1:
		return time.Minute
	case attempts == 2:
		return 5 * time.Minute
	default:
		return 30 * time.Minute
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

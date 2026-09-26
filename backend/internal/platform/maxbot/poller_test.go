package maxbot

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	maxapi "github.com/max-messenger/max-bot-api-client-go/v2"
	"github.com/max-messenger/max-bot-api-client-go/v2/model"
)

type step struct {
	updates []model.Update
	next    int64
	err     error
}

// fakeUpdates replays scripted responses and records the markers it was called with.
type fakeUpdates struct {
	mu      sync.Mutex
	steps   []step
	markers []int64
	done    func()
}

func (f *fakeUpdates) GetUpdates(ctx context.Context, marker int64) ([]model.Update, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.markers = append(f.markers, marker)
	if len(f.steps) == 0 {
		f.done()
		<-ctx.Done()

		return nil, marker, ctx.Err()
	}
	s := f.steps[0]
	f.steps = f.steps[1:]

	return s.updates, s.next, s.err
}

type recorder struct {
	mu  sync.Mutex
	got []string
}

func (r *recorder) Handle(_ context.Context, u model.Update) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if u.Payload == "panic" {
		panic("boom")
	}
	r.got = append(r.got, u.Payload)
}

func TestPollerOrderMarkersAndErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	upd := func(p string) model.Update { return model.Update{UpdateType: model.UpdateBotStarted, Payload: p} }
	f := &fakeUpdates{
		done: cancel,
		steps: []step{
			{updates: []model.Update{upd("a"), upd("b")}, next: 10},
			{err: &maxapi.TimeoutError{Op: "get updates"}},              // normal: no updates, no backoff
			{err: errors.New("network down")},                           // retried after backoff
			{updates: []model.Update{upd("panic"), upd("c")}, next: 11}, // a panic must not stop the loop
		},
	}
	rec := &recorder{}
	p := &Poller{
		Updates: f, Handler: rec, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		MinBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}

	finished := make(chan struct{})
	go func() { p.Run(ctx); close(finished) }()

	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("poller did not stop after context cancel")
	}

	if got := rec.got; len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("handled %v, want [a b c] in order", got)
	}
	// Marker advances only after successful responses.
	want := []int64{0, 10, 10, 10, 11}
	if len(f.markers) != len(want) {
		t.Fatalf("markers %v, want %v", f.markers, want)
	}
	for i := range want {
		if f.markers[i] != want[i] {
			t.Fatalf("markers %v, want %v", f.markers, want)
		}
	}
}

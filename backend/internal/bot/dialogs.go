package bot

import (
	"sync"
	"time"
)

// dialogs remembers what the bot waits for from a user after a button: the text of a
// question or of an answer. The state lives in memory: the bot runs as one copy per
// token (long polling), and a state lost on restart only means pressing the button
// again. It expires soon, so an unrelated message later is not sent as a question.
type dialogs struct {
	mu    sync.Mutex
	items map[int64]dialog // by MAX user id
}

type dialog struct {
	kind  string // dialogQuestion or dialogAnswer
	ref   string // the initiative for a question, the question for an answer
	until time.Time
}

const (
	dialogQuestion = "question"
	dialogAnswer   = "answer"

	dialogTTL = 15 * time.Minute
)

func (d *dialogs) set(maxUserID int64, kind, ref string, now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.items == nil {
		d.items = map[int64]dialog{}
	}
	for id, it := range d.items {
		if !now.Before(it.until) {
			delete(d.items, id) // few users wait at once: a sweep on write keeps the map small
		}
	}
	d.items[maxUserID] = dialog{kind: kind, ref: ref, until: now.Add(dialogTTL)}
}

// take returns the pending dialog of the user and forgets it.
func (d *dialogs) take(maxUserID int64, now time.Time) (dialog, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	it, ok := d.items[maxUserID]
	delete(d.items, maxUserID)
	if !ok || !now.Before(it.until) {
		return dialog{}, false
	}

	return it, true
}

func (d *dialogs) clear(maxUserID int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.items[maxUserID]
	delete(d.items, maxUserID)

	return ok
}

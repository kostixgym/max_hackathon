package bot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/poll"
	"maxhackathon/backend/internal/registry"
)

func resultJob(t *testing.T) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"initiative_id": testInitiative})
	if err != nil {
		t.Fatal(err)
	}

	return payload
}

func newPollResult(in fakeInitiatives, progress fakeProgress, accounts fakeAccounts) (*PollResult, *fakeMessages) {
	msgs := &fakeMessages{}

	return &PollResult{
		Messages: msgs, Initiatives: in, Houses: fakeHouseReader{}, Progress: progress, Accounts: accounts,
		Me: Identity{UserID: 555, Username: "dom_test_bot"}, Now: func() time.Time { return testNow },
	}, msgs
}

// When the term ends the initiator gets the numbers and whether a demand to the
// management company is possible (решения 18, 77).
func TestPollResultToInitiator(t *testing.T) {
	ended := fakeInitiatives{initiator: "user-42", stage: "poll", endsAt: testNow}

	result, msgs := newPollResult(ended, fakeProgress{forCenti: 124000}, fakeAccounts{})
	if err := result.HandleJob(context.Background(), resultJob(t)); err != nil {
		t.Fatal(err)
	}
	if len(msgs.sent) != 1 || msgs.sent[0].to != 1042 {
		t.Fatalf("sent = %+v, want one message to the initiator 1042", msgs.sent)
	}
	body := msgs.sent[0].body
	for _, want := range []string{"Опрос завершён", "За — <b>1240,00 м²</b> (41,3% площади дома)", "против — 48,00 м²",
		"Проголосовало собственников: 4", "Поддержки достаточно", "Камеры &amp; &lt;b&gt;домофон"} {
		if !strings.Contains(body.Text, want) {
			t.Errorf("text has no %q:\n%s", want, body.Text)
		}
	}
	if body.Format != model.FormatHTML || body.Attachments[0].Payload.Buttons[0][0].Type != model.ButtonOpenApp {
		t.Fatalf("body = %+v", body)
	}

	// 124 м² of 3 000: not enough for a demand.
	result, msgs = newPollResult(ended, fakeProgress{forCenti: 12400}, fakeAccounts{})
	if err := result.HandleJob(context.Background(), resultJob(t)); err != nil {
		t.Fatal(err)
	}
	if text := msgs.sent[0].body.Text; !strings.Contains(text, "поддержки не хватило") || !strings.Contains(text, "300,00 м²") {
		t.Fatalf("text = %s", text)
	}
}

func TestPollResultSkips(t *testing.T) {
	cases := map[string]struct {
		in       fakeInitiatives
		accounts fakeAccounts
	}{
		"moved on to a demand": {fakeInitiatives{initiator: "user-42", stage: "demand", endsAt: testNow}, fakeAccounts{}},
		"cancelled":            {fakeInitiatives{initiator: "user-42", stage: "canceled", endsAt: testNow}, fakeAccounts{}},
		"term moved later":     {fakeInitiatives{initiator: "user-42", stage: "poll", endsAt: testNow.Add(time.Hour)}, fakeAccounts{}},
		"initiator deleted":    {fakeInitiatives{initiator: "user-42", stage: "poll", endsAt: testNow}, fakeAccounts{deleted: "user-42"}},
		"no initiator":         {fakeInitiatives{stage: "poll", endsAt: testNow}, fakeAccounts{}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			result, msgs := newPollResult(c.in, fakeProgress{}, c.accounts)
			if err := result.HandleJob(context.Background(), resultJob(t)); err != nil || len(msgs.sent) != 0 {
				t.Fatalf("err = %v, sent = %d; want nothing and no retry", err, len(msgs.sent))
			}
		})
	}

	// The clock of the app may lag behind the database a little.
	result, msgs := newPollResult(fakeInitiatives{initiator: "user-42", stage: "poll", endsAt: testNow.Add(30 * time.Second)},
		fakeProgress{}, fakeAccounts{})
	if err := result.HandleJob(context.Background(), resultJob(t)); err != nil || len(msgs.sent) != 1 {
		t.Fatalf("slightly early: err = %v, sent = %d, want the result", err, len(msgs.sent))
	}
}

// Titles, addresses and numbers of premises come from users and the registry: none
// may become markup in a message of the HTML format.
func TestPollMessageEscapes(t *testing.T) {
	endsAt := testNow.Add(24 * time.Hour)
	view := pollView{
		initiative: initiatives.Polling{ID: testInitiative, Title: `<a href="https://evil">Камеры</a>`, Stage: "poll",
			PollEndsAt: &endsAt},
		house: registry.HouseRef{Address: "ул. Ленина, д. 1 & 2", InviteSlug: "demo-slug_1"},
		vote:  &poll.CastResult{Choice: poll.ChoiceAgainst, PremiseNumber: "<i>45</i>", WeightNum: 4800, WeightDen: 1},
		now:   testNow,
	}
	text, kb := view.render(Identity{UserID: 555, Username: "dom_test_bot"})

	if strings.Contains(text, "<a ") || strings.Contains(text, "<i>45") ||
		!strings.Contains(text, "&lt;a href=&#34;https://evil&#34;&gt;") || !strings.Contains(text, "д. 1 &amp; 2") {
		t.Fatalf("text = %s", text)
	}
	if !strings.Contains(text, "<mark>Ваш голос: «против», 48,00 м² (кв. &lt;i&gt;45&lt;/i&gt;)</mark>") {
		t.Fatalf("the vote line = %s", text)
	}
	if row := kb.Build().Payload.Buttons[0]; row[0].Text != "Поддерживаю" || row[1].Text != "✅ Против" {
		t.Fatalf("vote buttons = %q, %q", row[0].Text, row[1].Text)
	}
}

func TestSupportLine(t *testing.T) {
	below := supportLine(poll.Progress{TotalCenti: 300000, ForNum: 29999, ForDen: 1})
	if !strings.Contains(below, "<b>299,99 м²</b> из 300,00 м²") {
		t.Fatalf("below the threshold: %s", below)
	}
	// Exactly 10% is enough: «не менее 10%» (ст. 45 ч. 6 ЖК).
	at := supportLine(poll.Progress{TotalCenti: 300000, ForNum: 30000, ForDen: 1})
	if !strings.Contains(at, "не меньше 10%") || !strings.Contains(at, "Этого достаточно") {
		t.Fatalf("at the threshold: %s", at)
	}
}

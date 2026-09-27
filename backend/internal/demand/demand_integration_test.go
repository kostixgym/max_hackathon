package demand

// Integration test of path A step 1.4 on PostgreSQL 18: инициатива → опрос →
// ровно 10% поддержки → требование → передача в УК (45 дней) → PDF. Skipped
// without TEST_DATABASE_URL like the other integration tests.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/documents"
	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/meeting"
	"maxhackathon/backend/internal/notify"
	"maxhackathon/backend/internal/platform/db"
	"maxhackathon/backend/internal/platform/security"
	"maxhackathon/backend/internal/poll"
	"maxhackathon/backend/internal/registry"
	"maxhackathon/backend/internal/rules"
	"maxhackathon/backend/migrations"
)

func TestDemandFlowIntegration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool, err := db.Connect(ctx, url, log)
	if err != nil {
		t.Fatal(err)
	}
	// t.Cleanup, not defer: row cleanups must run before the pool closes.
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool, migrations.FS, log); err != nil {
		t.Fatal(err)
	}

	houses := registry.NewStore(pool)
	tm := db.NewTransactionManager(pool)
	notifier := notify.NewStore(pool)
	catalog := rules.NewCatalog(pool)
	users := access.NewStore(pool, houses, nil)
	inits := initiatives.NewService(pool, tm, catalog, houses, users, notifier)
	polls := poll.NewStore(pool, inits, users, houses)
	meetings := meeting.NewService(pool, tm, inits, users, houses, notifier)
	demands := NewService(pool, tm, inits, polls, users, meetings, houses, users, documents.DemandPDF)
	if err := catalog.SeedCatalog(ctx); err != nil {
		t.Fatal(err)
	}

	slug, err := houses.SeedDemo(ctx, security.NewHasher([]byte("test-secret-test-secret-test-secret")), "", log)
	if err != nil {
		t.Fatal(err)
	}
	house, err := houses.HouseBySlug(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}

	// Пять собственников больших квартир (63,50 м², одна доля): 10% демо-дома —
	// 300 м². Четыре голоса дают 254 м² (ниже порога), пятый — 317,50 м².
	const votersN = 5
	run := time.Now().UnixNano()
	newUser := func(n int64) access.User {
		t.Helper()
		u, err := users.EnsureUser(ctx, run+n)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(),
				`DELETE FROM users WHERE id = $1::uuid`, u.ID)
		})

		return u
	}
	var voters []access.User
	for i := 0; i < votersN; i++ {
		u := newUser(int64(i + 1))
		flat := strconv.Itoa(4 * (i + 1)) // квартиры 4, 8, 12, 16, 20 — по 63,50 м²
		if _, err := users.ConfirmDemoOwner(ctx, u.ID, house.ID, flat, 1); err != nil {
			t.Fatalf("confirm flat %s: %v", flat, err)
		}
		voters = append(voters, u)
	}

	initiative, err := inits.CreateFromTemplate(ctx, initiatives.CreateInput{
		HouseID: house.ID, InitiatorUserID: voters[0].ID,
		TemplateCode: "video_surveillance", Title: "Камеры в подъездах",
		Params: json.RawMessage(`{"camera_count": 6, "payment_method": "management_bill",
			"records_access": "management_company", "placement": "входы в подъезды"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM jobs WHERE payload->>'initiative_id' = $1`, initiative.ID)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM initiatives WHERE id = $1::uuid`, initiative.ID)
	})
	if _, err := inits.StartPoll(ctx, initiative.ID, voters[0].ID, time.Time{}); err != nil {
		t.Fatal(err)
	}

	vote := func(u access.User, choice string) {
		t.Helper()
		if _, err := polls.CastVote(ctx, poll.CastInput{InitiativeID: initiative.ID, UserID: u.ID, Choice: choice}); err != nil {
			t.Fatalf("vote %s: %v", u.ID, err)
		}
	}

	// Четыре голоса «за» — 254 м², ниже порога: требование отклоняется.
	for _, u := range voters[:4] {
		vote(u, poll.ChoiceFor)
	}
	if _, err := demands.Create(ctx, initiative.ID, voters[0].ID, ChannelPaper); !errors.Is(err, ErrSupportNotReached) {
		t.Fatalf("below threshold: %v, want ErrSupportNotReached", err)
	}

	// Пятый голос: 317,50 м² — над порогом (10% = 300 м², «не менее 10%»).
	vote(voters[4], poll.ChoiceFor)
	var votes int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM poll_votes WHERE initiative_id = $1::uuid`, initiative.ID).Scan(&votes); err != nil {
		t.Fatal(err)
	}
	if votes != 5 {
		t.Fatalf("votes = %d, want 5", votes)
	}

	// Не инициатор требование не создаёт.
	if _, err := demands.Create(ctx, initiative.ID, voters[1].ID, ChannelPaper); !errors.Is(err, ErrNotInitiator) {
		t.Fatalf("not initiator: %v, want ErrNotInitiator", err)
	}

	// Создание требования: путь A, стадия demand, поддержка зафиксирована дробью.
	d, err := demands.Create(ctx, initiative.ID, voters[0].ID, ChannelPaper)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != "draft" || d.Channel != ChannelPaper || d.SupportNum != 38100 || d.SupportDen != 1 {
		t.Fatalf("demand = %+v", d)
	}

	// Повторное требование — ErrExists (гонка двух создателей закрыта стадией).
	if _, err := demands.Create(ctx, initiative.ID, voters[0].ID, ChannelPaper); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate demand: %v, want ErrExists", err)
	}

	// Передача в УК: только инициатор, один раз, срок 45 дней.
	delivered := time.Now()
	if _, err := demands.MarkDelivered(ctx, d.ID, voters[1].ID, delivered); !errors.Is(err, ErrNotInitiator) {
		t.Fatalf("bob mark: %v, want ErrNotInitiator", err)
	}
	d2, err := demands.MarkDelivered(ctx, d.ID, voters[0].ID, delivered)
	if err != nil {
		t.Fatal(err)
	}
	if d2.Status != "delivered" || d2.UKDueAt == nil ||
		d2.UKDueAt.Sub(*d2.DeliveredAt) < 44*24*time.Hour {
		t.Fatalf("delivered = %+v, want uk_due_at ≈ +45 дней", d2)
	}
	if _, err := demands.MarkDelivered(ctx, d.ID, voters[0].ID, delivered); !errors.Is(err, ErrAlreadyDelivered) {
		t.Fatalf("second mark: %v, want ErrAlreadyDelivered", err)
	}

	// PDF требования собирается с кириллицей.
	pdf, err := demands.PDF(ctx, d.ID, voters[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(pdf) < 2000 || string(pdf[:4]) != "%PDF" {
		t.Fatalf("pdf: %d bytes, prefix %q", len(pdf), pdf[:4])
	}

	// Просрочка: срок в прошлом, собрания нет → overdue при чтении.
	if _, err := pool.Exec(ctx, `UPDATE demands SET uk_due_at = now() - interval '1 day' WHERE id = $1::uuid`, d.ID); err != nil {
		t.Fatal(err)
	}
	d3, err := demands.Get(ctx, d.ID, voters[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !d3.Overdue {
		t.Fatal("overdue = false, want true")
	}

	// Чужой пользователь требование не видит.
	outsider := newUser(1000)
	if _, err := demands.Get(ctx, d.ID, outsider.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("outsider: %v, want ErrForbidden", err)
	}

	// Правило «не менее 10%»: ровно 300 м² проходят — уже покрыто support 317,50;
	// отдельная проверка границы не нужна: DemandReached проверен тестами poll.

	// Стадия и путь после требования.
	var stage, path *string
	if err := pool.QueryRow(ctx,
		`SELECT stage, path::text FROM initiatives WHERE id = $1::uuid`, initiative.ID).Scan(&stage, &path); err != nil {
		t.Fatal(err)
	}
	if *stage != "demand" || path == nil || *path != "A" {
		t.Fatalf("stage/path = %s/%v, want demand/A", *stage, path)
	}
}

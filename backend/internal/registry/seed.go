package registry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"maxhackathon/backend/internal/platform/security"
)

const (
	demoAddress = "г. Казань, ул. Демонстрационная, д. 1 (демо-дом)"
	demoRegion  = "Республика Татарстан"
	demoOrgName = "ООО «Демо-УК»"
)

// SeedDemo creates the synthetic demo house if there is none yet and returns its invite slug.
// An empty slug makes the seed generate a random one. The seed is idempotent:
// an existing demo house is kept as is, so restarts never duplicate data.
//
// Several instances may start on an empty database at once: the check and the insert
// run in one transaction under an advisory lock, so they seed one after another and
// the later ones find the house. The unique index houses_one_demo backs this up in the schema.
func (s *Store) SeedDemo(ctx context.Context, hasher *security.Hasher, slug string, log *slog.Logger) (string, error) {
	var result string
	err := s.tx.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('registry.seed_demo'))`); err != nil {
			return err
		}

		var existing string
		err := tx.QueryRow(ctx, `SELECT invite_slug FROM houses WHERE is_demo`).Scan(&existing)
		switch {
		case err == nil:
			if slug != "" && slug != existing {
				log.Warn("demo house already exists with another invite slug; DEMO_INVITE_SLUG is ignored", "slug", existing)
			}
			result = existing

			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("find demo house: %w", err)
		}

		if slug == "" {
			if slug, err = security.NewToken(); err != nil {
				return err
			}
		}
		result = slug

		return seedDemo(ctx, tx, hasher, slug)
	})
	if err != nil {
		return "", fmt.Errorf("seed demo house: %w", err)
	}

	return result, nil
}

func seedDemo(ctx context.Context, tx pgx.Tx, hasher *security.Hasher, slug string) error {
	premises := DemoPremises()

	var orgID, houseID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO organizations (type, name) VALUES ('uk', $1) RETURNING id::text`, demoOrgName,
	).Scan(&orgID); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO houses (org_id, address, region, locality, street, house_number, timezone, passport_area_centi, invite_slug, is_demo)
		VALUES ($1, $2, $3, 'Казань', 'Демонстрационная', '1', 'Europe/Moscow', $4, $5, true) RETURNING id::text`,
		orgID, demoAddress, demoRegion, DemoTotalAreaCenti(premises), slug,
	).Scan(&houseID); err != nil {
		return err
	}

	const version = 1
	var uploadID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO registry_uploads (house_id, version, total_area_centi, status, report)
		VALUES ($1, $2, $3, 'preview', '{"source": "demo seed"}') RETURNING id::text`,
		houseID, version, DemoTotalAreaCenti(premises),
	).Scan(&uploadID); err != nil {
		return err
	}

	for _, p := range premises {
		var premiseID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO premises (house_id, number, kind, entrance, floor, account_hmac, display_area_centi)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id::text`,
			houseID, p.Number, p.Kind, p.Entrance, p.Floor, hasher.Account(p.Account), p.AreaCenti,
		).Scan(&premiseID); err != nil {
			return fmt.Errorf("premise %s: %w", p.Number, err)
		}

		for _, o := range p.Owners {
			w, err := NewWeight(p.AreaCenti, o.ShareNum, o.ShareDen)
			if err != nil {
				return fmt.Errorf("premise %s: %w", p.Number, err)
			}

			var phoneHMAC []byte
			if o.Phone != "" {
				if phoneHMAC, err = hasher.Phone(o.Phone); err != nil {
					return fmt.Errorf("premise %s: %w", p.Number, err)
				}
			}

			var ownerID string
			if err := tx.QueryRow(ctx,
				`INSERT INTO owners (premise_id) VALUES ($1) RETURNING id::text`, premiseID,
			).Scan(&ownerID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO owner_records (owner_id, registry_upload_id, full_name, share_num, share_den,
				                           weight_num, weight_den, owner_kind, phone_hmac)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
				ownerID, uploadID, o.FullName, o.ShareNum, o.ShareDen, w.Num, w.Den, o.Kind, phoneHMAC,
			); err != nil {
				return fmt.Errorf("premise %s owner: %w", p.Number, err)
			}
		}
	}

	return applyVersion(ctx, tx, houseID, version)
}

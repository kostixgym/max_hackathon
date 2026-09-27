package initiatives

import (
	"context"
	"fmt"
)

// Read models of the mini-app screens: the list of a house and the card.

// MaxListed limits the list of a house to the newest initiatives.
const MaxListed = 100

// ListByHouse returns the initiatives of the house the viewer may see, newest first.
// A draft is seen only by those who lead it: the neighbours see an initiative from the
// start of the poll (docs/04, решение 76). Hidden initiatives are not listed. Whether
// the viewer may see the house at all is the access module's question.
func (s *Service) ListByHouse(ctx context.Context, houseID, viewerID string) ([]Initiative, error) {
	result := make([]Initiative, 0)
	if !validID(houseID) || !validID(viewerID) {
		return result, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id::text, house_id::text, title, stage, path, poll_ends_at,
		       initiator_user_id::text, author_user_id::text, created_at
		FROM initiatives
		WHERE house_id = $1::uuid AND hidden_at IS NULL
		  AND (stage <> 'draft' OR initiator_user_id = $2::uuid OR author_user_id = $2::uuid)
		ORDER BY created_at DESC, id DESC
		LIMIT $3`, houseID, viewerID, MaxListed)
	if err != nil {
		return nil, fmt.Errorf("list initiatives: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var in Initiative
		if err := rows.Scan(&in.ID, &in.HouseID, &in.Title, &in.Stage, &in.Path, &in.PollEndsAt,
			&in.InitiatorUserID, &in.AuthorUserID, &in.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan initiative: %w", err)
		}
		result = append(result, in)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list initiatives: %w", err)
	}

	return result, nil
}

// Details returns the card of the initiative: its fields, the agenda with the majority
// of every question, the template version and the registry snapshot the votes and
// thresholds are counted by (решение 55). Who may see it is decided by the caller.
func (s *Service) Details(ctx context.Context, id string) (Initiative, error) {
	in, err := s.Get(ctx, id)
	if err != nil {
		return in, err
	}

	if in.AgendaItems, err = s.agenda(ctx, in.ID); err != nil {
		return in, err
	}

	if in.TemplateID != nil {
		tpl, err := s.templates.TemplateByID(ctx, *in.TemplateID)
		if err != nil {
			return in, err
		}
		in.Template = &TemplateRef{Code: tpl.Code, Name: tpl.Name, Version: tpl.Version}
	}

	snap, err := s.registry.Snapshot(ctx, in.RegistryUploadID)
	if err != nil {
		return in, err
	}
	in.RegistryVersion, in.TotalAreaCenti = snap.Version, snap.TotalAreaCenti

	return in, nil
}

// agenda reads the questions of the initiative; the majority rules come from the
// rules module catalog.
func (s *Service) agenda(ctx context.Context, initiativeID string) ([]AgendaItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, position, text, decision_type_id::text
		FROM agenda_items
		WHERE initiative_id = $1::uuid
		ORDER BY position`, initiativeID)
	if err != nil {
		return nil, fmt.Errorf("agenda: %w", err)
	}
	defer rows.Close()

	items := make([]AgendaItem, 0)
	typeIDs := make([]string, 0)
	for rows.Next() {
		var item AgendaItem
		var typeID string
		if err := rows.Scan(&item.ID, &item.Position, &item.Text, &typeID); err != nil {
			return nil, fmt.Errorf("scan agenda item: %w", err)
		}
		items = append(items, item)
		typeIDs = append(typeIDs, typeID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("agenda: %w", err)
	}
	if len(items) == 0 {
		return items, nil
	}

	types, err := s.templates.DecisionTypes(ctx, typeIDs)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]int, len(types))
	for i, t := range types {
		byID[t.ID] = i
	}
	for i := range items {
		t, ok := byID[typeIDs[i]]
		if !ok {
			return nil, fmt.Errorf("agenda item %d: decision type %s is missing", items[i].Position, typeIDs[i])
		}
		items[i].MajorityRule, items[i].LegalReference = types[t].MajorityRule, types[t].LegalReference
	}

	return items, nil
}

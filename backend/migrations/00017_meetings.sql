-- +goose Up
CREATE TABLE meetings (
    id                             uuid PRIMARY KEY DEFAULT uuidv7(),
    initiative_id                  uuid NOT NULL REFERENCES initiatives (id),
    attempt                        integer NOT NULL DEFAULT 1 CHECK (attempt > 0),
    form                           text NOT NULL CHECK (form IN ('gis_electronic', 'paper_absentee')),
    status                         text NOT NULL DEFAULT 'preparation' CHECK (status IN (
        'preparation', 'notice', 'voting', 'counting', 'completed', 'canceling', 'canceled'
    )),
    administrator_user_id          uuid REFERENCES users (id) ON DELETE SET NULL,
    chair_owner_id                 uuid NOT NULL REFERENCES owners (id),
    secretary_owner_id             uuid NOT NULL REFERENCES owners (id),
    notice_at                      timestamptz NOT NULL,
    voting_starts_at               timestamptz NOT NULL,
    voting_ends_at                 timestamptz NOT NULL,
    external_ref                   text,
    outcome                        text CHECK (outcome IN ('held', 'no_quorum')),
    online_participants_weight_num bigint CHECK (online_participants_weight_num >= 0),
    online_participants_weight_den bigint CHECK (online_participants_weight_den > 0),
    finalized_by_user_id           uuid REFERENCES users (id) ON DELETE SET NULL,
    finalized_at                   timestamptz,
    cancel_reason                  text,
    created_at                     timestamptz NOT NULL DEFAULT now(),
    updated_at                     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (initiative_id, attempt),
    CHECK (chair_owner_id <> secretary_owner_id),
    CHECK (voting_starts_at >= notice_at),
    CHECK (voting_ends_at > voting_starts_at),
    CHECK ((online_participants_weight_num IS NULL) = (online_participants_weight_den IS NULL)),
    CHECK (status <> 'completed' OR (outcome IS NOT NULL AND finalized_at IS NOT NULL))
);

CREATE UNIQUE INDEX meetings_one_active_per_initiative
    ON meetings (initiative_id)
    WHERE status NOT IN ('completed', 'canceled');
CREATE UNIQUE INDEX meetings_external_ref_unique
    ON meetings (external_ref)
    WHERE external_ref IS NOT NULL;

-- +goose Down
DROP TABLE meetings;

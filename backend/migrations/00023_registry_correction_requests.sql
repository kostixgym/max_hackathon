-- +goose Up
CREATE TABLE registry_correction_requests (
    id                 uuid PRIMARY KEY DEFAULT uuidv7(),
    membership_id      uuid REFERENCES memberships (id) ON DELETE SET NULL,
    user_id            uuid REFERENCES users (id) ON DELETE SET NULL,
    house_id           uuid NOT NULL REFERENCES houses (id),
    premise_id         uuid NOT NULL,
    registry_upload_id uuid NOT NULL,
    reason             text NOT NULL CHECK (reason IN (
        'wrong_name', 'wrong_share', 'wrong_area', 'wrong_premise', 'other'
    )),
    comment            text,
    status             text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'resolved', 'rejected')),
    handled_by_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    resolution         text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    handled_at         timestamptz,
    FOREIGN KEY (house_id, premise_id)
        REFERENCES premises (house_id, id),
    FOREIGN KEY (house_id, registry_upload_id)
        REFERENCES registry_uploads (house_id, id),
    CHECK (status = 'pending' OR handled_at IS NOT NULL)
);

CREATE INDEX registry_correction_requests_house_status_idx
    ON registry_correction_requests (house_id, status, created_at);
CREATE UNIQUE INDEX registry_correction_requests_one_pending_reason
    ON registry_correction_requests (membership_id, reason)
    WHERE status = 'pending' AND membership_id IS NOT NULL;

-- +goose Down
DROP TABLE registry_correction_requests;

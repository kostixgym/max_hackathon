-- +goose Up
CREATE TABLE premises (
    id                 uuid PRIMARY KEY DEFAULT uuidv7(),
    house_id           uuid NOT NULL REFERENCES houses (id),
    number             text NOT NULL,
    kind               text NOT NULL CHECK (kind IN ('residential', 'nonresidential', 'parking')),
    entrance           integer,
    floor              integer,
    account_hmac       bytea,
    display_area_centi bigint CHECK (display_area_centi > 0),
    UNIQUE (house_id, number),
    UNIQUE (house_id, id)
);

CREATE INDEX premises_account_hmac_idx
    ON premises (account_hmac)
    WHERE account_hmac IS NOT NULL;

-- +goose Down
DROP TABLE premises;

-- +goose Up
ALTER TABLE houses
    ADD COLUMN locality text,
    ADD COLUMN street text,
    ADD COLUMN house_number text,
    ADD COLUMN building text,
    ADD COLUMN structure text;

-- The demo address is known and can be migrated without guessing at free-form
-- addresses entered for other houses.
UPDATE houses
SET locality = 'Казань',
    street = 'Демонстрационная',
    house_number = '1'
WHERE is_demo
  AND locality IS NULL
  AND address LIKE 'г. Казань, ул. Демонстрационная, д. 1%';

CREATE INDEX houses_locality_idx ON houses (lower(locality)) WHERE locality IS NOT NULL;
CREATE INDEX houses_street_idx ON houses (lower(street)) WHERE street IS NOT NULL;

-- +goose Down
DROP INDEX houses_street_idx;
DROP INDEX houses_locality_idx;
ALTER TABLE houses
    DROP COLUMN structure,
    DROP COLUMN building,
    DROP COLUMN house_number,
    DROP COLUMN street,
    DROP COLUMN locality;

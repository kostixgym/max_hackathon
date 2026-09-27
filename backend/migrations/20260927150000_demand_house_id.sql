-- +goose Up
-- Дом требования денормализован: кабинету УК нужен список требований по домам
-- без JOIN на чужую таблицу инициатив (docs/04, принцип 8).
ALTER TABLE demands ADD COLUMN house_id uuid REFERENCES houses (id);
UPDATE demands d SET house_id = i.house_id FROM initiatives i WHERE i.id = d.initiative_id;
ALTER TABLE demands ALTER COLUMN house_id SET NOT NULL;
CREATE INDEX demands_house_idx ON demands (house_id);

-- +goose Down
ALTER TABLE demands DROP COLUMN house_id;

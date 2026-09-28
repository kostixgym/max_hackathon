-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX IF NOT EXISTS houses_address_trgm_idx ON houses USING gin (address gin_trgm_ops);
CREATE INDEX IF NOT EXISTS houses_region_trgm_idx ON houses USING gin (region gin_trgm_ops);
CREATE INDEX IF NOT EXISTS houses_locality_trgm_idx ON houses USING gin (locality gin_trgm_ops);
CREATE INDEX IF NOT EXISTS houses_street_trgm_idx ON houses USING gin (street gin_trgm_ops);
CREATE INDEX IF NOT EXISTS houses_house_number_trgm_idx ON houses USING gin (house_number gin_trgm_ops);
CREATE INDEX IF NOT EXISTS houses_building_trgm_idx ON houses USING gin (building gin_trgm_ops);
CREATE INDEX IF NOT EXISTS houses_structure_trgm_idx ON houses USING gin (structure gin_trgm_ops);

-- +goose Down
DROP INDEX houses_structure_trgm_idx;
DROP INDEX houses_building_trgm_idx;
DROP INDEX houses_house_number_trgm_idx;
DROP INDEX houses_street_trgm_idx;
DROP INDEX houses_locality_trgm_idx;
DROP INDEX houses_region_trgm_idx;
DROP INDEX houses_address_trgm_idx;

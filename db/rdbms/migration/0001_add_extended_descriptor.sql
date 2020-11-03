-- +goose Up
ALTER TABLE jobs ADD COLUMN extended_descriptor text;

-- +goose Down
ALTER TABLE jobs DROP COLUMN extended_descriptor;

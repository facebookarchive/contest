-- +goose Up
ALTER TABLE jobs ADD COLUMN extended_descriptor text;

-- +goose Down
-- Down migration is not supported and it's a no-op

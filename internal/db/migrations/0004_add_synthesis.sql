-- +goose Up
ALTER TABLE investigations
    ADD COLUMN synthesis JSONB;

-- +goose Down
ALTER TABLE investigations
    DROP COLUMN synthesis;

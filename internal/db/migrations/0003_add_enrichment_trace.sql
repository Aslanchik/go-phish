-- +goose Up
ALTER TABLE investigations
    ADD COLUMN enrichment_trace   JSONB,
    ADD COLUMN enrichment_summary TEXT;

-- +goose Down
ALTER TABLE investigations
    DROP COLUMN enrichment_trace,
    DROP COLUMN enrichment_summary;

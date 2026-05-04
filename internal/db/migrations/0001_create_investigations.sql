-- +goose Up
CREATE TABLE investigations (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    url             TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    status          TEXT        NOT NULL DEFAULT 'pending',
    error_message   TEXT,

    -- Phase 1 artifacts
    final_url       TEXT,
    rendered_dom    TEXT,
    screenshot      BYTEA,
    network_log     JSONB,
    js_files        JSONB,
    forms           JSONB,

    -- Phase 2 output
    hypothesis      JSONB,

    -- Final report
    report          TEXT
);

-- +goose Down
DROP TABLE investigations;

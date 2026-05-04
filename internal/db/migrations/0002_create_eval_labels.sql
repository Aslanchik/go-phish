-- +goose Up
CREATE TABLE eval_labels (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    investigation_id     UUID        NOT NULL REFERENCES investigations(id),
    labeled_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    labeled_by           TEXT,
    brand_impersonated   TEXT        NOT NULL,
    exfil_destination    TEXT,
    kit_name             TEXT,
    is_actually_phishing BOOLEAN     NOT NULL
);

-- +goose Down
DROP TABLE eval_labels;

-- +goose Up
CREATE TABLE events (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    api_key_id      uuid        NOT NULL REFERENCES api_keys (id) ON DELETE RESTRICT,
    type            text        NOT NULL CHECK (char_length(type) BETWEEN 1 AND 200),
    payload         jsonb       NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    idempotency_key text        NOT NULL CHECK (char_length(idempotency_key) BETWEEN 1 AND 128),
    occurred_at     timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT events_api_key_id_idempotency_key_key UNIQUE (api_key_id, idempotency_key)
);

CREATE INDEX events_api_key_id_created_at_idx ON events (api_key_id, created_at);

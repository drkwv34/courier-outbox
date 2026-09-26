-- +goose Up
CREATE TABLE subscriptions (
    id                 uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    api_key_id         uuid        NOT NULL REFERENCES api_keys (id) ON DELETE RESTRICT,
    target_url         text        NOT NULL,
    event_types        jsonb       NOT NULL DEFAULT '["*"]'::jsonb CHECK (jsonb_typeof(event_types) = 'array'),
    headers            jsonb       NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(headers) = 'object'),
    signing_secret_enc bytea       NOT NULL,
    enabled            boolean     NOT NULL DEFAULT true,
    description        text        NOT NULL DEFAULT '',
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX subscriptions_api_key_id_created_at_idx ON subscriptions (api_key_id, created_at);

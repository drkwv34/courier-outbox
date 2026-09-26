-- +goose Up
CREATE TABLE api_keys (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    prefix      text        NOT NULL CHECK (prefix ~ '^[a-z2-7]{8}$'),
    key_hash    bytea       NOT NULL CHECK (octet_length(key_hash) = 32),
    created_at  timestamptz NOT NULL DEFAULT now(),
    revoked_at  timestamptz,
    CONSTRAINT api_keys_prefix_key UNIQUE (prefix)
);

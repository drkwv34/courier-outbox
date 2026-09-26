-- +goose Up
CREATE TABLE delivery_attempts (
    id            bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    delivery_id   uuid        NOT NULL REFERENCES deliveries (id) ON DELETE CASCADE,
    status_code   integer     CHECK (status_code BETWEEN 100 AND 599),
    error_message text,
    duration_ms   integer     NOT NULL CHECK (duration_ms >= 0),
    request_id    text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT delivery_attempts_outcome_chk CHECK (status_code IS NOT NULL OR error_message IS NOT NULL)
);

CREATE INDEX delivery_attempts_delivery_id_created_at_idx ON delivery_attempts (delivery_id, created_at);

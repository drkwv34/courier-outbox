-- +goose Up
CREATE TABLE deliveries (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id        uuid        NOT NULL REFERENCES events (id) ON DELETE CASCADE,
    subscription_id uuid        NOT NULL REFERENCES subscriptions (id) ON DELETE RESTRICT,
    status          text        NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'retrying', 'delivered', 'dead_lettered')),
    attempt_count   integer     NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_owner     text,
    lease_until     timestamptz,
    delivered_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT deliveries_event_id_subscription_id_key UNIQUE (event_id, subscription_id),
    CONSTRAINT deliveries_lease_pair_chk CHECK ((lease_owner IS NULL) = (lease_until IS NULL)),
    CONSTRAINT deliveries_delivered_at_chk CHECK ((status = 'delivered') = (delivered_at IS NOT NULL))
);

CREATE INDEX deliveries_status_next_attempt_at_idx ON deliveries (status, next_attempt_at);
CREATE INDEX deliveries_subscription_id_created_at_idx ON deliveries (subscription_id, created_at);

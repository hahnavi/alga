-- +goose Up
CREATE TABLE counters (
    id TEXT PRIMARY KEY,
    seq BIGINT NOT NULL DEFAULT 0 CHECK (seq >= 0)
);

-- +goose Down
DROP TABLE IF EXISTS counters;

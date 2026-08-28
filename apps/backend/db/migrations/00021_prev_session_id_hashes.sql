-- refresh-token family reuse detection needs rotated-out session IDs
-- to stay recognizable for the sliding-expiry window, so a replayed old cookie
-- can be detected and the whole session family revoked. Sessions already track
-- previous refresh-token hashes; this adds the symmetric history for session-ID
-- hashes (bounded in code, like prev_refresh_token_hashes).

-- +goose Up
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS prev_session_id_hashes JSONB NULL;

-- +goose Down
ALTER TABLE sessions DROP COLUMN IF EXISTS prev_session_id_hashes;

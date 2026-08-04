-- Derived statistics about each user's posts, computed asynchronously by stats_service.
-- Kept out of the users table on purpose: this is computed data with its own job state,
-- so a stale or failed calculation never corrupts the users source of truth.
CREATE TABLE IF NOT EXISTS user_post_stats (
  users_id         INTEGER PRIMARY KEY,
  total_characters BIGINT      NOT NULL DEFAULT 0,
  posts_count      INTEGER     NOT NULL DEFAULT 0,
  -- pending | done | failed
  status           TEXT        NOT NULL DEFAULT 'pending',
  request_id       TEXT,
  error_message    TEXT,
  requested_at     TIMESTAMPTZ,
  calculated_at    TIMESTAMPTZ,
  CONSTRAINT fk_user_post_stats_user FOREIGN KEY(users_id) REFERENCES users(id) ON DELETE CASCADE
);

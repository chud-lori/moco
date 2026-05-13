-- Adds a Google OAuth identity column. NULL = the user has no linked
-- Google account (they signed up via email/password only). The partial
-- UNIQUE index allows multiple NULLs but enforces one row per Google sub
-- so we can look up users by sub at login time.
--
-- password_hash stays NOT NULL — callers store "" for Google-only users.
-- VerifyPassword rejects an empty hash, so an OAuth-only account can
-- never be logged into via the password endpoint.
ALTER TABLE users ADD COLUMN google_sub TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_google_sub
  ON users(google_sub) WHERE google_sub IS NOT NULL;

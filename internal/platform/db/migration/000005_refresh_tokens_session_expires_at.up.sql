-- session_expires_at caps a login's lifetime. An untrusted device's login gets
-- one (login + UNTRUSTED_SESSION_DURATION), copied to every successor, and no
-- refresh token outlives it. NULL is a trusted device: refresh tokens keep
-- sliding with no cap (rows from before this migration stay that way).
ALTER TABLE refresh_tokens
    ADD COLUMN session_expires_at timestamptz;

-- auth_sessions is one login on one device: one row per refresh-token family. Login inserts it, refresh
-- touches it, logout / remote sign-out / a password change / reuse detection revokes it.
CREATE TABLE auth_sessions
(
    family_id     uuid PRIMARY KEY,
    user_id       bigint       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    trusted       boolean      NOT NULL,
    user_agent    varchar(512) NOT NULL DEFAULT '',
    client_ip     varchar(64)  NOT NULL DEFAULT '',
    created_at    timestamptz  NOT NULL DEFAULT now(),
    last_used_at  timestamptz  NOT NULL DEFAULT now(),
    revoked_at    timestamptz,
    revoke_reason varchar(32)
        CONSTRAINT auth_sessions_revoke_reason_check CHECK (revoke_reason IN
            ('logout', 'signed_out_remotely', 'password_changed', 'password_reset', 'token_reused', 'deactivated')),
    CONSTRAINT auth_sessions_revoked_fields CHECK ((revoked_at IS NULL) = (revoke_reason IS NULL))
);

CREATE INDEX auth_sessions_user_id_idx ON auth_sessions (user_id) WHERE revoked_at IS NULL;

-- Families from before this table have no session row, so they are revoked here: every live login has a
-- session, and everyone signs in again once.
UPDATE refresh_tokens SET revoked_at = now() WHERE revoked_at IS NULL;

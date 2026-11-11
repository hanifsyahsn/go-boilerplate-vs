ALTER TABLE users ADD COLUMN email_verified_at timestamptz;

-- auth_tokens are one-time tokens sent by email; only the SHA-256 is stored.
-- VERIFY_EMAIL lives 24 h, RESET_PASSWORD 1 h. Issuing a new one spends the user's unused ones of that purpose.
CREATE TABLE auth_tokens
(
    id         bigserial PRIMARY KEY,
    purpose    varchar(16) NOT NULL
        CONSTRAINT auth_tokens_purpose_check CHECK (purpose IN ('VERIFY_EMAIL', 'RESET_PASSWORD')),
    user_id    bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash varchar     NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    used_at    timestamptz
);

CREATE UNIQUE INDEX auth_tokens_token_hash_key ON auth_tokens (token_hash);
CREATE INDEX auth_tokens_user_id_idx ON auth_tokens (user_id, purpose) WHERE used_at IS NULL;

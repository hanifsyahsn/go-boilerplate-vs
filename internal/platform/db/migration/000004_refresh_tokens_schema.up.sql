-- refresh_tokens makes refresh tokens rotating and revocable. Every refresh
-- token's jti is one row. A login starts a family; each refresh revokes the
-- presented token (replaced_by = its successor) and issues the next one in the
-- same family. Presenting an already-rotated token again means it leaked, so
-- the whole family is revoked. Logout revokes the family too.
CREATE TABLE refresh_tokens
(
    jti         uuid PRIMARY KEY,
    user_id     bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    family_id   uuid        NOT NULL,
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    revoked_at  timestamptz,
    replaced_by uuid
);

CREATE INDEX refresh_tokens_family_id_idx ON refresh_tokens (family_id);
CREATE INDEX refresh_tokens_user_id_idx ON refresh_tokens (user_id);

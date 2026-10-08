CREATE TABLE users
(
    id            bigserial PRIMARY KEY,
    name          varchar(255) NOT NULL,
    email         varchar(255) NOT NULL,
    password_hash varchar      NOT NULL,
    role          varchar(32)  NOT NULL DEFAULT 'user'
        CONSTRAINT users_role_check CHECK (role IN ('user', 'admin')),
    is_active     boolean      NOT NULL DEFAULT true,
    created_at    timestamptz  NOT NULL DEFAULT now(),
    updated_at    timestamptz  NOT NULL DEFAULT now()
);

-- One account per email, ignoring case.
CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email));

CREATE TRIGGER users_updated_at
    BEFORE UPDATE
    ON users
    FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

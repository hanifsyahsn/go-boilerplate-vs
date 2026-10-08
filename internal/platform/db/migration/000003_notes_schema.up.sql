-- notes is the example domain: every slice under internal/features/note
-- reads or writes it. A note belongs to its creator and is soft-deleted.
CREATE TABLE notes
(
    id         bigserial PRIMARY KEY,
    title      varchar(255) NOT NULL,
    body       text         NOT NULL DEFAULT '',
    created_by bigint       NOT NULL REFERENCES users (id),
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    deleted_by bigint REFERENCES users (id)
);

-- The list reads a creator's live notes, newest first.
CREATE INDEX notes_created_by_live_idx ON notes (created_by, created_at DESC) WHERE deleted_at IS NULL;

CREATE TRIGGER notes_updated_at
    BEFORE UPDATE
    ON notes
    FOR EACH ROW
EXECUTE PROCEDURE update_updated_at_column();

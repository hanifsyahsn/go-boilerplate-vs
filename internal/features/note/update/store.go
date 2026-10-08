package update

import (
	"context"
	"database/sql"

	notesharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/update/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/db/tx"
)

// Querier is what the update transaction runs: the slice's own queries plus
// the note-shared ones (lock + read-back), bound to the same *sql.Tx.
type Querier interface {
	sqlc.Querier
	notesharedsqlc.Querier
}

// StoreI owns the transaction boundary; the business logic stays in the
// service.
type StoreI interface {
	RunInTx(ctx context.Context, fn func(q Querier) error) error
}

type Store struct {
	db      *sql.DB
	queries *sqlc.Queries
	shared  *notesharedsqlc.Queries
}

func NewStore(db *sql.DB) StoreI {
	return &Store{db: db, queries: sqlc.New(db), shared: notesharedsqlc.New(db)}
}

// txQueries binds both querier sets to one transaction. The shared one is
// embedded under a local alias so the embedded field names don't clash.
type sharedQuerier = notesharedsqlc.Queries

type txQueries struct {
	*sqlc.Queries
	*sharedQuerier
}

func (s *Store) RunInTx(ctx context.Context, fn func(q Querier) error) error {
	return tx.Exec(ctx, s.db, func(t *sql.Tx) Querier {
		return txQueries{Queries: s.queries.WithTx(t), sharedQuerier: s.shared.WithTx(t)}
	}, fn)
}

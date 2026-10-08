package delete

import (
	"context"
	"database/sql"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/delete/sqlc"
	notesharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/note/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/db/tx"
)

// Querier is what the delete transaction runs: the slice's own queries plus
// the note-shared lock, bound to the same *sql.Tx.
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

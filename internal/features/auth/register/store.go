package register

import (
	"context"
	"database/sql"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/register/sqlc"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/db/tx"
)

// Querier is what the register transaction runs: the slice's queries plus the
// auth-shared token queries, bound to the same *sql.Tx.
type Querier interface {
	sqlc.Querier
	authsharedsqlc.Querier
}

// StoreI owns the transaction boundary; the business logic stays in the
// service.
type StoreI interface {
	RunInTx(ctx context.Context, fn func(q Querier) error) error
}

type Store struct {
	db      *sql.DB
	queries *sqlc.Queries
	shared  *authsharedsqlc.Queries
}

func NewStore(db *sql.DB) StoreI {
	return &Store{db: db, queries: sqlc.New(db), shared: authsharedsqlc.New(db)}
}

// The shared one is embedded under a local alias so the embedded field names
// don't clash.
type sharedQuerier = authsharedsqlc.Queries

type txQueries struct {
	*sqlc.Queries
	*sharedQuerier
}

func (s *Store) RunInTx(ctx context.Context, fn func(q Querier) error) error {
	return tx.Exec(ctx, s.db, func(t *sql.Tx) Querier {
		return txQueries{Queries: s.queries.WithTx(t), sharedQuerier: s.shared.WithTx(t)}
	}, fn)
}

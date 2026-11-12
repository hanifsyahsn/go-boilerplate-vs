package logout

import (
	"context"
	"database/sql"

	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/db/tx"
)

// StoreI owns the transaction that ends the session: the session row is
// locked, then the session and its tokens are revoked (authshared.EndSession),
// and the lock holds until commit.
type StoreI interface {
	RunInTx(ctx context.Context, fn func(q authsharedsqlc.Querier) error) error
}

type Store struct {
	db     *sql.DB
	shared *authsharedsqlc.Queries
}

func NewStore(db *sql.DB) StoreI {
	return &Store{db: db, shared: authsharedsqlc.New(db)}
}

func (s *Store) RunInTx(ctx context.Context, fn func(q authsharedsqlc.Querier) error) error {
	return tx.Exec(ctx, s.db, func(t *sql.Tx) authsharedsqlc.Querier { return s.shared.WithTx(t) }, fn)
}

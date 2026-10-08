package tx

import (
	"context"
	"database/sql"
	"fmt"
)

// DBExecutor *sql.DB satisfies the interface
type DBExecutor interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

type QueriesFactory[Q any] func(tx *sql.Tx) Q

func Exec[Q any](
	ctx context.Context,
	db DBExecutor,
	newQueries QueriesFactory[Q],
	fn func(q Q) error,
) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	q := newQueries(tx)

	if err = fn(q); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx err: %v, rb err: %v", err, rbErr)
		}
		return err
	}
	return tx.Commit()
}

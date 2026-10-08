package tx

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestTx_Executor(t *testing.T) {
	// identity factory: Q = *sql.Tx, so the "queries" object IS the tx itself.
	// Keeps this test self-contained without depending on any generated sqlc package.
	identityFactory := func(tx *sql.Tx) *sql.Tx {
		return tx
	}

	testCases := []struct {
		name           string
		callSetup      func(ctx context.Context, db DBExecutor) error
		mockSetup      func(sqlmock.Sqlmock)
		expectedResult func(error)
	}{
		{
			name: "success - execute transaction",
			callSetup: func(ctx context.Context, db DBExecutor) error {
				return Exec(
					ctx,
					db,
					identityFactory,
					func(tx *sql.Tx) error {
						return nil
					},
				)
			},
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectBegin()
				sqlMock.ExpectCommit()
			},
			expectedResult: func(err error) {
				require.NoError(t, err)
			},
		},
		{
			name: "failed - db.BeginTx() fails",
			callSetup: func(ctx context.Context, db DBExecutor) error {
				return Exec(
					ctx,
					db,
					identityFactory,
					func(tx *sql.Tx) error {
						return nil
					},
				)
			},
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectBegin().WillReturnError(sql.ErrConnDone)
			},
			expectedResult: func(err error) {
				require.ErrorContains(t, err, "connection is already closed")
			},
		},
		{
			name: "failed - fn(tx) failed, rollback succeeds",
			callSetup: func(ctx context.Context, db DBExecutor) error {
				return Exec(
					ctx,
					db,
					identityFactory,
					func(tx *sql.Tx) error {
						return errors.New("something went wrong")
					},
				)
			},
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectBegin()
				sqlMock.ExpectRollback()
			},
			expectedResult: func(err error) {
				require.EqualError(t, err, "something went wrong")
			},
		},
		{
			name: "failed - fn(tx) failed, rollback also fails",
			callSetup: func(ctx context.Context, db DBExecutor) error {
				return Exec(
					ctx,
					db,
					identityFactory,
					func(tx *sql.Tx) error {
						return errors.New("something went wrong")
					},
				)
			},
			mockSetup: func(sqlMock sqlmock.Sqlmock) {
				sqlMock.ExpectBegin()
				sqlMock.ExpectRollback().WillReturnError(errors.New("rollback failed"))
			},
			expectedResult: func(err error) {
				require.ErrorContains(t, err, "tx err")
				require.ErrorContains(t, err, "rb err")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			require.NoError(t, err)

			ctx := context.Background()

			tc.mockSetup(sqlMock)

			err = tc.callSetup(ctx, db)
			tc.expectedResult(err)
		})
	}
}

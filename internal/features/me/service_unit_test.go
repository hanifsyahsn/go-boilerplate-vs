package me

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/me/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/stretchr/testify/require"
)

func TestService_Get_U(t *testing.T) {
	const userID = int64(7)
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)

	testCases := []struct {
		name     string
		row      sqlc.GetMeRow
		readErr  error
		want     MeResource
		wantCode apperror.Code
	}{
		{
			name: "the caller's profile",
			row:  sqlc.GetMeRow{ID: userID, Name: "Jane", Email: "jane@example.com", Role: "admin", CreatedAt: now},
			want: MeResource{ID: "7", Name: "Jane", Email: "jane@example.com", Role: "admin", CreatedAt: now},
		},
		{name: "a missing user is a 404", readErr: sql.ErrNoRows, wantCode: apperror.CodeNotFound},
		{name: "a read error is a 500", readErr: sql.ErrConnDone, wantCode: apperror.CodeInternal},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			q.EXPECT().GetMe(gomock.Any(), userID).Return(tc.row, tc.readErr)

			got, err := NewService(q).Get(context.Background(), userID)
			if tc.wantCode != "" {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

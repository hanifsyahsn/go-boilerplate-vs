package list

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/list/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/session/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/stretchr/testify/require"
)

func TestService_List_U(t *testing.T) {
	now := time.Now()
	current, other := uuid.New(), uuid.New()
	capAt := now.Add(time.Hour)
	rows := []sqlc.ListLiveSessionsRow{
		{FamilyID: current, UserAgent: "Laptop", ClientIp: "10.0.0.1", Trusted: true, CreatedAt: now, LastUsedAt: now},
		{FamilyID: other, UserAgent: "Phone", ClientIp: "10.0.0.2", CreatedAt: now, LastUsedAt: now,
			SessionExpiresAt: sql.NullTime{Time: capAt, Valid: true}},
	}

	testCases := []struct {
		name     string
		stub     func(q *sqlc.MockQuerier)
		want     []shared.SessionResource
		wantCode apperror.Code
	}{
		{
			name: "the caller's live sessions in the asked order, the request's own marked current, an untrusted one's cap as expires_at",
			stub: func(q *sqlc.MockQuerier) {
				q.EXPECT().ListLiveSessions(gomock.Any(), sqlc.ListLiveSessionsParams{UserID: 7, Sort: "-last_used_at"}).Return(rows, nil)
			},
			want: []shared.SessionResource{
				{ID: current.String(), UserAgent: "Laptop", ClientIP: "10.0.0.1", Trusted: true, CreatedAt: now, LastUsedAt: now, Current: true},
				{ID: other.String(), UserAgent: "Phone", ClientIP: "10.0.0.2", CreatedAt: now, LastUsedAt: now, ExpiresAt: &capAt},
			},
		},
		{
			name: "no live session: an empty list, not nil",
			stub: func(q *sqlc.MockQuerier) {
				q.EXPECT().ListLiveSessions(gomock.Any(), gomock.Any()).Return([]sqlc.ListLiveSessionsRow{}, nil)
			},
			want: []shared.SessionResource{},
		},
		{
			name: "a read error is a 500",
			stub: func(q *sqlc.MockQuerier) {
				q.EXPECT().ListLiveSessions(gomock.Any(), gomock.Any()).Return(nil, sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			tc.stub(q)

			got, err := NewService(q).List(context.Background(), 7, current, "-last_used_at")
			if !tc.wantCode.IsZero() {
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

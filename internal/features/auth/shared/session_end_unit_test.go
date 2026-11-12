package shared

import (
	"context"
	"database/sql"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/stretchr/testify/require"
)

func TestEndSession_U(t *testing.T) {
	familyID := uuid.New()
	revoke := authsharedsqlc.RevokeSessionParams{FamilyID: familyID, Reason: RevokeReasonLogout}

	testCases := []struct {
		name    string
		stub    func(q *authsharedsqlc.MockQuerier)
		want    int64
		wantErr string
	}{
		{
			name: "the session row is locked before the session and its tokens are revoked",
			stub: func(q *authsharedsqlc.MockQuerier) {
				gomock.InOrder(
					q.EXPECT().LockSession(gomock.Any(), familyID).Return(nil),
					q.EXPECT().RevokeSession(gomock.Any(), revoke).Return(int64(1), nil),
				)
			},
			want: 1,
		},
		{
			name: "a lock error stops before anything is revoked",
			stub: func(q *authsharedsqlc.MockQuerier) {
				q.EXPECT().LockSession(gomock.Any(), familyID).Return(sql.ErrConnDone)
			},
			wantErr: "LockSession(" + familyID.String() + ")",
		},
		{
			name: "a revoke error is returned",
			stub: func(q *authsharedsqlc.MockQuerier) {
				q.EXPECT().LockSession(gomock.Any(), familyID).Return(nil)
				q.EXPECT().RevokeSession(gomock.Any(), revoke).Return(int64(0), sql.ErrConnDone)
			},
			wantErr: "RevokeSession(" + familyID.String() + ")",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			q := authsharedsqlc.NewMockQuerier(gomock.NewController(t))
			tc.stub(q)
			n, err := EndSession(context.Background(), q, familyID, RevokeReasonLogout)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.ErrorIs(t, err, sql.ErrConnDone)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, n)
		})
	}
}

func TestEndUserSessions_U(t *testing.T) {
	keep := uuid.NullUUID{UUID: uuid.New(), Valid: true}

	testCases := []struct {
		name    string
		keep    uuid.NullUUID
		stub    func(q *authsharedsqlc.MockQuerier, keep uuid.NullUUID)
		want    int64
		wantErr string
	}{
		{
			name: "every session: the user's session rows locked, then revoked",
			stub: func(q *authsharedsqlc.MockQuerier, keep uuid.NullUUID) {
				gomock.InOrder(
					q.EXPECT().LockUserSessions(gomock.Any(), authsharedsqlc.LockUserSessionsParams{UserID: 7}).Return(nil),
					q.EXPECT().RevokeUserSessions(gomock.Any(), authsharedsqlc.RevokeUserSessionsParams{
						UserID: 7, Reason: RevokeReasonPasswordReset,
					}).Return(int64(3), nil),
				)
			},
			want: 3,
		},
		{
			name: "all but one: the kept session is left out of the lock and the revoke",
			keep: keep,
			stub: func(q *authsharedsqlc.MockQuerier, keep uuid.NullUUID) {
				gomock.InOrder(
					q.EXPECT().LockUserSessions(gomock.Any(), authsharedsqlc.LockUserSessionsParams{UserID: 7, KeepFamilyID: keep}).Return(nil),
					q.EXPECT().RevokeUserSessions(gomock.Any(), authsharedsqlc.RevokeUserSessionsParams{
						UserID: 7, Reason: RevokeReasonPasswordReset, KeepFamilyID: keep,
					}).Return(int64(2), nil),
				)
			},
			want: 2,
		},
		{
			name: "a lock error stops before anything is revoked",
			stub: func(q *authsharedsqlc.MockQuerier, _ uuid.NullUUID) {
				q.EXPECT().LockUserSessions(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
			},
			wantErr: "LockUserSessions(7)",
		},
		{
			name: "a revoke error is returned",
			stub: func(q *authsharedsqlc.MockQuerier, _ uuid.NullUUID) {
				q.EXPECT().LockUserSessions(gomock.Any(), gomock.Any()).Return(nil)
				q.EXPECT().RevokeUserSessions(gomock.Any(), gomock.Any()).Return(int64(0), sql.ErrConnDone)
			},
			wantErr: "RevokeUserSessions(7)",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			q := authsharedsqlc.NewMockQuerier(gomock.NewController(t))
			tc.stub(q, tc.keep)
			n, err := EndUserSessions(context.Background(), q, 7, tc.keep, RevokeReasonPasswordReset)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.ErrorIs(t, err, sql.ErrConnDone)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, n)
		})
	}
}

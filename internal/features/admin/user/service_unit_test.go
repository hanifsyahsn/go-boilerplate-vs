package user

import (
	"context"
	"database/sql"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/admin/user/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/pagination"
	"github.com/stretchr/testify/require"
)

func TestService_List_U(t *testing.T) {
	active := true

	testCases := []struct {
		name      string
		req       ListRequest
		stub      func(q *sqlc.MockQuerier)
		wantItems []UserResource
		wantMeta  pagination.Meta
		wantCode  apperror.Code
	}{
		{
			name: "filters passed through",
			req:  ListRequest{Q: " jane ", IsActive: &active, Sort: "name"},
			stub: func(q *sqlc.MockQuerier) {
				search := sql.NullString{String: "jane", Valid: true}
				flag := sql.NullBool{Bool: true, Valid: true}
				q.EXPECT().ListUsers(gomock.Any(), sqlc.ListUsersParams{Q: search, IsActive: flag, Sort: "name", Limit: 20}).
					Return([]sqlc.ListUsersRow{{ID: 3, Name: "Jane", IsActive: true}}, nil)
				q.EXPECT().CountUsers(gomock.Any(), sqlc.CountUsersParams{Q: search, IsActive: flag}).Return(int64(1), nil)
			},
			wantItems: []UserResource{{ID: "3", Name: "Jane", IsActive: true}},
			wantMeta:  pagination.Meta{Page: 1, PageSize: 20, Total: 1, TotalPage: 1},
		},
		{
			name: "a list error is a 500",
			stub: func(q *sqlc.MockQuerier) {
				q.EXPECT().ListUsers(gomock.Any(), gomock.Any()).Return(nil, sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name: "a count error is a 500",
			stub: func(q *sqlc.MockQuerier) {
				q.EXPECT().ListUsers(gomock.Any(), gomock.Any()).Return([]sqlc.ListUsersRow{}, nil)
				q.EXPECT().CountUsers(gomock.Any(), gomock.Any()).Return(int64(0), sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			tc.stub(q)

			items, meta, err := NewService(q, nil).List(context.Background(), tc.req)
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantItems, items)
			require.Equal(t, tc.wantMeta, meta)
		})
	}
}

func TestService_Update_U(t *testing.T) {
	yes, no := true, false

	testCases := []struct {
		name     string
		userID   int64
		isActive *bool
		stub     func(q *sqlc.MockQuerier, tq *MockQuerier)
		want     UserResource
		wantCode apperror.Code
	}{
		{
			name: "nothing to change: the user as it stands", userID: 3, isActive: nil,
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				q.EXPECT().GetUser(gomock.Any(), int64(3)).Return(sqlc.GetUserRow{ID: 3, Name: "Jane", IsActive: true}, nil)
			},
			want: UserResource{ID: "3", Name: "Jane", IsActive: true},
		},
		{
			name: "nothing to change, missing user: 404", userID: 9, isActive: nil,
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				q.EXPECT().GetUser(gomock.Any(), int64(9)).Return(sqlc.GetUserRow{}, sql.ErrNoRows)
			},
			wantCode: apperror.CodeNotFound,
		},
		{
			name: "nothing to change, read error: 500", userID: 3, isActive: nil,
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				q.EXPECT().GetUser(gomock.Any(), int64(3)).Return(sqlc.GetUserRow{}, sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name: "deactivate another user: every session of theirs ends (deactivated) in the same transaction", userID: 3, isActive: &no,
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				gomock.InOrder(
					tq.EXPECT().SetUserActive(gomock.Any(), sqlc.SetUserActiveParams{ID: 3, IsActive: false}).
						Return(sqlc.SetUserActiveRow{ID: 3, Name: "Jane", IsActive: false}, nil),
					tq.EXPECT().RevokeUserSessions(gomock.Any(), authsharedsqlc.RevokeUserSessionsParams{
						UserID: 3, Reason: authshared.RevokeReasonDeactivated,
					}).Return(int64(2), nil),
				)
			},
			want: UserResource{ID: "3", Name: "Jane", IsActive: false},
		},
		{
			name: "re-activating yourself is allowed", userID: 1, isActive: &yes,
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				tq.EXPECT().SetUserActive(gomock.Any(), sqlc.SetUserActiveParams{ID: 1, IsActive: true}).
					Return(sqlc.SetUserActiveRow{ID: 1, IsActive: true}, nil) // activating revokes nothing
			},
			want: UserResource{ID: "1", IsActive: true},
		},
		{name: "deactivating yourself is a 409", userID: 1, isActive: &no, stub: func(*sqlc.MockQuerier, *MockQuerier) {}, wantCode: apperror.CodeConflict},
		{
			name: "a missing user is a 404", userID: 9, isActive: &no,
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				tq.EXPECT().SetUserActive(gomock.Any(), gomock.Any()).Return(sqlc.SetUserActiveRow{}, sql.ErrNoRows)
			},
			wantCode: apperror.CodeNotFound,
		},
		{
			name: "a write error is a 500", userID: 3, isActive: &yes,
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				tq.EXPECT().SetUserActive(gomock.Any(), gomock.Any()).Return(sqlc.SetUserActiveRow{}, sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name: "a session revoke error is a 500 (the transaction rolls the deactivation back)", userID: 3, isActive: &no,
			stub: func(q *sqlc.MockQuerier, tq *MockQuerier) {
				tq.EXPECT().SetUserActive(gomock.Any(), gomock.Any()).Return(sqlc.SetUserActiveRow{ID: 3}, nil)
				tq.EXPECT().RevokeUserSessions(gomock.Any(), gomock.Any()).Return(int64(0), sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := sqlc.NewMockQuerier(ctrl)
			tq := NewMockQuerier(ctrl)
			tc.stub(q, tq)
			store := NewMockStoreI(ctrl)
			store.EXPECT().RunInTx(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, fn func(Querier) error) error { return fn(tq) }).AnyTimes()

			got, err := NewService(q, store).Update(context.Background(), tc.userID, UpdateRequest{IsActive: tc.isActive}, 1)
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

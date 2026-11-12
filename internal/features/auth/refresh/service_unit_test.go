package refresh

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/refresh/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	internalshareddto "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/dto"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/enum"
	"github.com/stretchr/testify/require"
)

func TestService_Refresh_U(t *testing.T) {
	const (
		accessTTL  = 15 * time.Minute
		refreshTTL = 7 * 24 * time.Hour
	)
	now := time.Now()
	jti := uuid.New()
	familyID := uuid.New()
	claims := jwt.MapClaims{"sub": float64(1), "jti": jti.String()}

	live := sqlc.LockRefreshTokenRow{Jti: jti, UserID: 1, FamilyID: familyID, SessionLive: true}
	rotatedAgo := func(d time.Duration) sqlc.LockRefreshTokenRow {
		row := live
		row.RevokedAt = sql.NullTime{Time: now.Add(-d), Valid: true}
		row.ReplacedBy = uuid.NullUUID{UUID: uuid.New(), Valid: true}
		return row
	}
	cappedIn := func(d time.Duration) sqlc.LockRefreshTokenRow {
		row := live
		row.SessionExpiresAt = sql.NullTime{Time: now.Add(d), Valid: true}
		return row
	}
	// Signed out (logout, a remote sign-out, a password change): the session
	// and its tokens revoked together.
	signedOut := live
	signedOut.RevokedAt = sql.NullTime{Time: now.Add(-time.Second), Valid: true}
	signedOut.SessionLive = false
	client := authshared.SessionClient{UserAgent: "Mozilla/5.0 (test)", IP: "203.0.113.7"}
	touch := sqlc.TouchSessionParams{FamilyID: familyID, UserAgent: "Mozilla/5.0 (test)", ClientIp: "203.0.113.7"}
	reused := authsharedsqlc.RevokeSessionParams{FamilyID: familyID, Reason: authshared.RevokeReasonTokenReused}

	active := sqlc.GetUserForRefreshRow{ID: 1, Name: "Jane", Email: "jane@example.com", Role: "admin", IsActive: true}
	inactive := active
	inactive.IsActive = false

	pair := jwttoken.Pair{
		AccessToken:           "acc",
		AccessTokenExpiresAt:  now.Add(accessTTL),
		RefreshToken:          "ref-2",
		RefreshTokenID:        uuid.New(),
		RefreshTokenExpiresAt: now.Add(refreshTTL),
	}

	// verified stubs a valid token up to its locked row.
	verified := func(q *MockQuerier, m *jwttoken.MockM, row sqlc.LockRefreshTokenRow) {
		m.EXPECT().VerifyRefreshToken("ref").Return(claims, nil)
		q.EXPECT().LockRefreshToken(gomock.Any(), jti).Return(row, nil)
	}

	testCases := []struct {
		name     string
		token    string
		stub     func(q *MockQuerier, m *jwttoken.MockM)
		want     Result
		wantCode apperror.Code
		// wantDetail, when set, is the error's user-facing message.
		wantDetail string
	}{
		{
			name:  "a live token: a new pair with the live role and the same session as sid, same family, the old token rotated, the session's use recorded",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, live)
				gomock.InOrder(
					q.EXPECT().GetUserForRefresh(gomock.Any(), int64(1)).Return(active, nil),
					m.EXPECT().CreateToken(internalshareddto.User{ID: 1, Name: "Jane", Email: "jane@example.com", Role: enum.RoleAdmin}, familyID, accessTTL, refreshTTL).
						Return(pair, nil),
					q.EXPECT().InsertRefreshToken(gomock.Any(), authsharedsqlc.InsertRefreshTokenParams{
						Jti: pair.RefreshTokenID, UserID: 1, FamilyID: familyID, ExpiresAt: pair.RefreshTokenExpiresAt,
					}).Return(nil),
					q.EXPECT().RotateRefreshToken(gomock.Any(), sqlc.RotateRefreshTokenParams{Jti: jti, ReplacedBy: pair.RefreshTokenID}).Return(nil),
					q.EXPECT().TouchSession(gomock.Any(), touch).Return(nil),
				)
			},
			want: Result{Tokens: pair},
		},
		{
			name:  "an untrusted device's token near its cap: the new refresh token ends at the cap, the cap carried over",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, cappedIn(time.Hour))
				q.EXPECT().GetUserForRefresh(gomock.Any(), int64(1)).Return(active, nil)
				m.EXPECT().CreateToken(gomock.Any(), familyID, accessTTL, time.Hour).Return(pair, nil)
				q.EXPECT().InsertRefreshToken(gomock.Any(), authsharedsqlc.InsertRefreshTokenParams{
					Jti: pair.RefreshTokenID, UserID: 1, FamilyID: familyID, ExpiresAt: pair.RefreshTokenExpiresAt,
					SessionExpiresAt: sql.NullTime{Time: now.Add(time.Hour), Valid: true},
				}).Return(nil)
				q.EXPECT().RotateRefreshToken(gomock.Any(), gomock.Any()).Return(nil)
				q.EXPECT().TouchSession(gomock.Any(), touch).Return(nil)
			},
			want: Result{Tokens: pair},
		},
		{
			name:  "an untrusted device's token far from its cap: the full refresh duration",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, cappedIn(30*24*time.Hour))
				q.EXPECT().GetUserForRefresh(gomock.Any(), int64(1)).Return(active, nil)
				m.EXPECT().CreateToken(gomock.Any(), familyID, accessTTL, refreshTTL).Return(pair, nil)
				q.EXPECT().InsertRefreshToken(gomock.Any(), gomock.Any()).Return(nil)
				q.EXPECT().RotateRefreshToken(gomock.Any(), gomock.Any()).Return(nil)
				q.EXPECT().TouchSession(gomock.Any(), touch).Return(nil)
			},
			want: Result{Tokens: pair},
		},
		{
			name:  "an untrusted device's session past its cap: 401, nothing issued",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, cappedIn(-time.Second))
			},
			wantCode: apperror.CodeUnauthorized,
		},
		{name: "no token", token: "", stub: func(q *MockQuerier, m *jwttoken.MockM) {}, wantCode: apperror.CodeUnauthorized},
		{
			name:  "an invalid token",
			token: "bad",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				m.EXPECT().VerifyRefreshToken("bad").Return(nil, errors.New("expired"))
			},
			wantCode: apperror.CodeUnauthorized,
		},
		{
			name:  "claims without a usable sub (defensive): 401 session expired",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				m.EXPECT().VerifyRefreshToken("ref").Return(jwt.MapClaims{"jti": jti.String()}, nil)
			},
			wantCode:   apperror.CodeUnauthorized,
			wantDetail: sessionExpiredMessage,
		},
		{
			name:  "a jti that isn't a uuid",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				m.EXPECT().VerifyRefreshToken("ref").Return(jwt.MapClaims{"sub": float64(1), "jti": "abc"}, nil)
			},
			wantCode: apperror.CodeUnauthorized,
		},
		{
			name:  "an unknown token (no row)",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				m.EXPECT().VerifyRefreshToken("ref").Return(claims, nil)
				q.EXPECT().LockRefreshToken(gomock.Any(), jti).Return(sqlc.LockRefreshTokenRow{}, sql.ErrNoRows)
			},
			wantCode: apperror.CodeUnauthorized,
		},
		{
			name:  "a lock error is a 500",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				m.EXPECT().VerifyRefreshToken("ref").Return(claims, nil)
				q.EXPECT().LockRefreshToken(gomock.Any(), jti).Return(sqlc.LockRefreshTokenRow{}, sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name:  "a row of another user",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				other := live
				other.UserID = 2
				verified(q, m, other)
			},
			wantCode: apperror.CodeUnauthorized,
		},
		{
			name:  "a spent token presented again: the session is revoked as token_reused",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, rotatedAgo(time.Minute))
				q.EXPECT().RevokeSession(gomock.Any(), reused).Return(int64(1), nil)
			},
			wantCode: apperror.CodeUnauthorized,
		},
		{
			name:  "a token rotated within the grace window (concurrent refresh): 401, the family kept",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, rotatedAgo(time.Second))
			},
			wantCode: apperror.CodeUnauthorized,
		},
		{
			name:  "a token of a signed-out session: 401 'your session has ended', nothing issued or revoked again",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, signedOut)
				q.EXPECT().GetUserForRefresh(gomock.Any(), int64(1)).Return(active, nil)
			},
			wantCode:   apperror.CodeUnauthorized,
			wantDetail: middleware.SessionEndedMessage,
		},
		{
			name:  "a signed-out session of a deactivated account: 401 'no longer active', like the access-token check",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, signedOut)
				q.EXPECT().GetUserForRefresh(gomock.Any(), int64(1)).Return(inactive, nil)
			},
			wantCode:   apperror.CodeUnauthorized,
			wantDetail: accountInactiveMessage,
		},
		{
			name:  "a signed-out session of a deleted account: 401 'no longer active'",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, signedOut)
				q.EXPECT().GetUserForRefresh(gomock.Any(), int64(1)).Return(sqlc.GetUserForRefreshRow{}, sql.ErrNoRows)
			},
			wantCode:   apperror.CodeUnauthorized,
			wantDetail: accountInactiveMessage,
		},
		{
			name:  "a signed-out session whose user read fails: 500",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, signedOut)
				q.EXPECT().GetUserForRefresh(gomock.Any(), int64(1)).Return(sqlc.GetUserForRefreshRow{}, sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name:  "a revoke error is a 500",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, rotatedAgo(time.Minute))
				q.EXPECT().RevokeSession(gomock.Any(), reused).Return(int64(0), sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name:  "a deactivated user",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, live)
				q.EXPECT().GetUserForRefresh(gomock.Any(), int64(1)).Return(inactive, nil)
			},
			wantCode: apperror.CodeUnauthorized,
		},
		{
			name:  "a deleted user",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, live)
				q.EXPECT().GetUserForRefresh(gomock.Any(), int64(1)).Return(sqlc.GetUserForRefreshRow{}, sql.ErrNoRows)
			},
			wantCode: apperror.CodeUnauthorized,
		},
		{
			name:  "a user read error is a 500",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, live)
				q.EXPECT().GetUserForRefresh(gomock.Any(), int64(1)).Return(sqlc.GetUserForRefreshRow{}, sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name:  "a signing error is a 500",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, live)
				q.EXPECT().GetUserForRefresh(gomock.Any(), int64(1)).Return(active, nil)
				m.EXPECT().CreateToken(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(jwttoken.Pair{}, errors.New("sign"))
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name:  "an insert error is a 500",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, live)
				q.EXPECT().GetUserForRefresh(gomock.Any(), int64(1)).Return(active, nil)
				m.EXPECT().CreateToken(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(pair, nil)
				q.EXPECT().InsertRefreshToken(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name:  "a rotate error is a 500",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, live)
				q.EXPECT().GetUserForRefresh(gomock.Any(), int64(1)).Return(active, nil)
				m.EXPECT().CreateToken(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(pair, nil)
				q.EXPECT().InsertRefreshToken(gomock.Any(), gomock.Any()).Return(nil)
				q.EXPECT().RotateRefreshToken(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
		{
			name:  "a touch error is a 500",
			token: "ref",
			stub: func(q *MockQuerier, m *jwttoken.MockM) {
				verified(q, m, live)
				q.EXPECT().GetUserForRefresh(gomock.Any(), int64(1)).Return(active, nil)
				m.EXPECT().CreateToken(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(pair, nil)
				q.EXPECT().InsertRefreshToken(gomock.Any(), gomock.Any()).Return(nil)
				q.EXPECT().RotateRefreshToken(gomock.Any(), gomock.Any()).Return(nil)
				q.EXPECT().TouchSession(gomock.Any(), gomock.Any()).Return(sql.ErrConnDone)
			},
			wantCode: apperror.CodeInternal,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			q := NewMockQuerier(ctrl)
			m := jwttoken.NewMockM(ctrl)
			tc.stub(q, m)

			store := NewMockStoreI(ctrl)
			store.EXPECT().RunInTx(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, fn func(Querier) error) error { return fn(q) }).AnyTimes()

			svc := NewService(store, m, accessTTL, refreshTTL)
			svc.(*service).now = func() time.Time { return now }

			got, err := svc.Refresh(context.Background(), tc.token, client)
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				if tc.wantDetail != "" {
					require.Equal(t, tc.wantDetail, appErr.Message)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

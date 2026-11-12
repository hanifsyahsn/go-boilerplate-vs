package shared

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/utility"
)

// Why a session (auth_sessions.revoke_reason) ended; the column's CHECK
// allows exactly these.
const (
	RevokeReasonLogout            = "logout"
	RevokeReasonSignedOutRemotely = "signed_out_remotely"
	RevokeReasonPasswordChanged   = "password_changed"
	RevokeReasonPasswordReset     = "password_reset"
	RevokeReasonTokenReused       = "token_reused"
	RevokeReasonDeactivated       = "deactivated"
)

// Column sizes of auth_sessions, in characters.
const (
	userAgentMaxRunes = 512
	clientIPMaxRunes  = 64
)

// SessionClient is the device a session is used from, as stored on the
// session row: the User-Agent and IP cut to their column sizes.
type SessionClient struct {
	UserAgent string
	IP        string
}

// NewSessionClient cuts the request's User-Agent and client IP to the
// auth_sessions columns (varchar counts characters, so runes, never bytes:
// a cut through a multi-byte character would be invalid UTF-8).
func NewSessionClient(userAgent, ip string) SessionClient {
	return SessionClient{UserAgent: utility.TruncateRunes(userAgent, userAgentMaxRunes), IP: utility.TruncateRunes(ip, clientIPMaxRunes)}
}

// EndSession locks a session row, then revokes the session and its tokens
// (RevokeSession). Run it inside a transaction: the lock holds until commit.
//
// Lock order: every path that touches a session and its tokens locks the
// session row first (refresh too), so a revoke and a refresh never wait on
// each other in opposite orders (a deadlock, a 500). The revoke runs in a
// later statement than the lock, so it sees a token a concurrent refresh
// rotated in and committed while this one waited: no live token is left
// behind under a revoked session. The count is the sessions revoked.
func EndSession(ctx context.Context, q authsharedsqlc.Querier, familyID uuid.UUID, reason string) (int64, error) {
	if err := q.LockSession(ctx, familyID); err != nil {
		return 0, fmt.Errorf("LockSession(%s): %w", familyID, err)
	}
	n, err := q.RevokeSession(ctx, authsharedsqlc.RevokeSessionParams{FamilyID: familyID, Reason: reason})
	if err != nil {
		return 0, fmt.Errorf("RevokeSession(%s): %w", familyID, err)
	}
	return n, nil
}

// EndUserSessions is EndSession for every live session of a user except
// keepFamilyID (uuid.NullUUID{} = all): the rows are locked in family_id
// order, then revoked. Run it inside a transaction.
func EndUserSessions(ctx context.Context, q authsharedsqlc.Querier, userID int64, keepFamilyID uuid.NullUUID, reason string) (int64, error) {
	if err := q.LockUserSessions(ctx, authsharedsqlc.LockUserSessionsParams{UserID: userID, KeepFamilyID: keepFamilyID}); err != nil {
		return 0, fmt.Errorf("LockUserSessions(%d): %w", userID, err)
	}
	n, err := q.RevokeUserSessions(ctx, authsharedsqlc.RevokeUserSessionsParams{UserID: userID, Reason: reason, KeepFamilyID: keepFamilyID})
	if err != nil {
		return 0, fmt.Errorf("RevokeUserSessions(%d): %w", userID, err)
	}
	return n, nil
}

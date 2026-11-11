package shared

import "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/utility"

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

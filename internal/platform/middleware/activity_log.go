package middleware

import (
	"math"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/requestid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/utility"
)

// Activity is one /api request, as the activity log keeps it (decision
// 0008). No body or query string: they can hold passwords and tokens.
type Activity struct {
	OccurredAt    time.Time // request start
	RequestID     string
	UserID        int64     // 0 = not signed in
	SessionID     uuid.UUID // uuid.Nil = not signed in
	Method        string
	Route         string // c.FullPath(), "" when none matched
	Path          string // c.Request.URL.Path (no query string)
	Status        int
	Latency       time.Duration
	ResponseBytes int
	ClientIP      string
	UserAgent     string
	APIClient     string // APIKey's client name, "" = not a machine call (never the key)
}

// ActivityRecorder takes an Activity off the request path. Record must never
// block (a full queue drops).
type ActivityRecorder interface{ Record(Activity) }

// The api_activity_logs column sizes, in characters.
const (
	requestIDMaxRunes = 128
	methodMaxRunes    = 10
	routeMaxRunes     = 255
	pathMaxRunes      = 2048
	clientIPMaxRunes  = 64
	userAgentMaxRunes = 512
	apiClientMaxRunes = 100
)

// healthzPath is the liveness probe, polled too often to be worth a row.
const healthzPath = "/healthz"

// ActivityLog hands every request (404 / 405 / panics included: it runs right
// after AccessLog, outside recovery) to rec once it is answered. The caller
// comes from the gin keys AccessTokenMiddleware sets, a machine caller from
// the one APIKey sets. Values longer than
// their column are cut to that many runes; latency and size clamp to int32.
// /healthz isn't recorded.
func ActivityLog(rec ActivityRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		if path == healthzPath {
			c.Next()
			return
		}

		c.Next()

		userID := c.GetInt64(internalsharedconstant.UserIDKey)
		sessionID, _ := c.Value(internalsharedconstant.SessionIDKey).(uuid.UUID)
		rec.Record(Activity{
			OccurredAt:    start,
			RequestID:     utility.TruncateRunes(c.Writer.Header().Get(requestid.Header), requestIDMaxRunes),
			UserID:        userID,
			SessionID:     sessionID,
			Method:        utility.TruncateRunes(c.Request.Method, methodMaxRunes),
			Route:         utility.TruncateRunes(c.FullPath(), routeMaxRunes),
			Path:          utility.TruncateRunes(path, pathMaxRunes),
			Status:        c.Writer.Status(),
			Latency:       time.Since(start),
			ResponseBytes: min(max(c.Writer.Size(), 0), math.MaxInt32),
			ClientIP:      utility.TruncateRunes(c.ClientIP(), clientIPMaxRunes),
			UserAgent:     utility.TruncateRunes(c.Request.UserAgent(), userAgentMaxRunes),
			APIClient:     utility.TruncateRunes(c.GetString(APIClientKey), apiClientMaxRunes),
		})
	}
}

package requestuser

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/stretchr/testify/require"
)

func TestFromContext_U(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name   string
		ctx    func() context.Context
		wantID int64
		wantOK bool
	}{
		{name: "nil ctx", ctx: func() context.Context { return nil }},
		{name: "outside a request", ctx: context.Background},
		{name: "set with With", ctx: func() context.Context { return With(context.Background(), 42) }, wantID: 42, wantOK: true},
		{
			name: "a gin context with the auth middleware's key",
			ctx: func() context.Context {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Set(internalsharedconstant.UserIdKey, int64(7))
				return c
			},
			wantID: 7, wantOK: true,
		},
		{
			name: "a gin key of the wrong type is ignored",
			ctx: func() context.Context {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Set(internalsharedconstant.UserIdKey, "7")
				return c
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			id, ok := FromContext(tc.ctx())
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.wantID, id)
		})
	}
}

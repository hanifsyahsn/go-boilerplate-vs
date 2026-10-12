package requestid

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFromContext_U(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name   string
		ctx    func() context.Context
		wantID string
	}{
		{name: "nil ctx", ctx: func() context.Context { return nil }, wantID: ""},
		{name: "no id", ctx: context.Background, wantID: ""},
		{name: "an id set with With", ctx: func() context.Context { return With(context.Background(), "abc") }, wantID: "abc"},
		{
			name: "a gin context with the id under Key",
			ctx: func() context.Context {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Set(Key, "from-gin")
				return c
			},
			wantID: "from-gin",
		},
		{
			name:   "a plain string key also works",
			ctx:    func() context.Context { return context.WithValue(context.Background(), Key, "untyped") }, //nolint:staticcheck // what a *gin.Context exposes
			wantID: "untyped",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wantID, FromContext(tc.ctx()))
		})
	}
}

func TestDetach_U(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name   string
		parent func() (context.Context, context.CancelFunc)
		wantID string
	}{
		{
			name: "keeps the id and drops the cancellation",
			parent: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(With(context.Background(), "abc"))
			},
			wantID: "abc",
		},
		{
			name: "from a gin context",
			parent: func() (context.Context, context.CancelFunc) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Set(Key, "from-gin")
				c.Set("user_id", int64(1))
				return c, func() {}
			},
			wantID: "from-gin",
		},
		{
			name:   "no id: a plain background context",
			parent: func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
			wantID: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parent, cancel := tc.parent()
			detached := Detach(parent)
			cancel()

			require.Equal(t, tc.wantID, FromContext(detached))
			require.NoError(t, detached.Err())
			require.Nil(t, detached.Value("user_id"), "carries only the id")
		})
	}
}

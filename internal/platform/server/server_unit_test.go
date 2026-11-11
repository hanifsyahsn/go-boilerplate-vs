package server

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/config"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	middlewaresqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/stretchr/testify/require"
)

func TestNewServer_U(t *testing.T) {
	testCases := []struct {
		name    string
		sec     Security
		wantErr string
	}{
		{name: "a valid security setup: the server listens on SERVER_ADDRESS",
			sec: Security{TrustedProxies: []string{"10.0.0.1"}, AuthIPLimit: ratelimit.NewMemory(ratelimit.Rate{Burst: 1, Per: time.Minute}),
				PublicIPLimit: ratelimit.NewMemory(ratelimit.Rate{Burst: 1, Per: time.Minute})}},
		{name: "a security setup the router refuses: no server, the error passed on (main stops)",
			sec: Security{}, wantErr: "AuthIPLimit is required"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			srv, err := NewServer(config.Config{ServerAddress: ":9999"}, "v", jwttoken.NewMockM(ctrl),
				middlewaresqlc.NewMockQuerier(ctrl), newMocks(ctrl).handlers(), tc.sec)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Nil(t, srv)
				return
			}
			require.NoError(t, err)
			require.Equal(t, ":9999", srv.httpServer.Addr)
			require.NotNil(t, srv.httpServer.Handler)
		})
	}
}

func TestGinMode_U(t *testing.T) {
	testCases := []struct {
		name        string
		environment string
		want        string
	}{
		{name: "development keeps gin's debug mode", environment: "development", want: gin.DebugMode},
		{name: "production runs in release mode", environment: "production", want: gin.ReleaseMode},
		{name: "any other environment (staging) runs in release mode", environment: "staging", want: gin.ReleaseMode},
		{name: "a different spelling isn't development: release, the safe side", environment: "Development", want: gin.ReleaseMode},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, GinMode(tc.environment))
		})
	}
}

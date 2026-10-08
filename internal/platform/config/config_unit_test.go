package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var validEnv = map[string]string{
	"ENVIRONMENT":            "test",
	"SERVER_ADDRESS":         ":8080",
	"DB_DRIVER":              "postgres",
	"DB_SOURCE":              "postgres://u:p@localhost:5432/db?sslmode=disable",
	"ACCESS_TOKEN_DURATION":  "15m",
	"REFRESH_TOKEN_DURATION": "168h",
	"TOKEN_ISSUER":           "issuer",
	"CLIENT_ID":              "client",
	"EC_PRIVATE_KEY_PATH":    "private.pem",
	"EC_PUBLIC_KEY_PATH":     "public.pem",
}

func TestLoadConfig_Env_U(t *testing.T) {
	testCases := []struct {
		name      string
		override  map[string]string
		wantErr   string
		wantCheck func(t *testing.T, c Config)
	}{
		{
			name: "every required var set: defaults applied to the optional ones",
			wantCheck: func(t *testing.T, c Config) {
				require.Equal(t, "test", c.Environment)
				require.Equal(t, 15*time.Minute, c.AccessTokenDuration)
				require.Equal(t, 168*time.Hour, c.RefreshTokenDuration)
				require.True(t, c.CookieSecure)
				require.Empty(t, c.CookieDomain)
			},
		},
		{
			name:     "no refresh duration: 7 days",
			override: map[string]string{"REFRESH_TOKEN_DURATION": ""},
			wantCheck: func(t *testing.T, c Config) {
				require.Equal(t, 7*24*time.Hour, c.RefreshTokenDuration)
			},
		},
		{
			name:     "a refresh duration read",
			override: map[string]string{"REFRESH_TOKEN_DURATION": "720h"},
			wantCheck: func(t *testing.T, c Config) {
				require.Equal(t, 720*time.Hour, c.RefreshTokenDuration)
			},
		},
		{
			name:     "optional cookie knobs read",
			override: map[string]string{"COOKIE_SECURE": "false", "COOKIE_DOMAIN": "example.com"},
			wantCheck: func(t *testing.T, c Config) {
				require.False(t, c.CookieSecure)
				require.Equal(t, "example.com", c.CookieDomain)
			},
		},
		{name: "a missing required var", override: map[string]string{"CLIENT_ID": ""}, wantErr: "CLIENT_ID is required"},
		{name: "an invalid duration", override: map[string]string{"ACCESS_TOKEN_DURATION": "soon"}, wantErr: "ACCESS_TOKEN_DURATION"},
		{name: "an invalid refresh duration", override: map[string]string{"REFRESH_TOKEN_DURATION": "later"}, wantErr: "REFRESH_TOKEN_DURATION"},
		{name: "an invalid bool", override: map[string]string{"COOKIE_SECURE": "maybe"}, wantErr: "COOKIE_SECURE"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range validEnv {
				t.Setenv(k, v)
			}
			t.Setenv("COOKIE_SECURE", "")
			t.Setenv("COOKIE_DOMAIN", "")
			for k, v := range tc.override {
				t.Setenv(k, v)
			}

			c, err := LoadConfig(t.TempDir())
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			tc.wantCheck(t, c)
		})
	}
}

func TestLoadConfig_EnvFile_U(t *testing.T) {
	testCases := []struct {
		name    string
		content string
		wantErr bool
	}{
		{name: "a .env file is read", content: "ENVIRONMENT=from-file\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range validEnv {
				if k != "ENVIRONMENT" {
					t.Setenv(k, v)
				}
			}
			t.Setenv("ENVIRONMENT", "")
			_ = os.Unsetenv("ENVIRONMENT")

			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(tc.content), 0o600))

			c, err := LoadConfig(dir)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "from-file", c.Environment)
		})
	}
}

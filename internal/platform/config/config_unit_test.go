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
				require.Equal(t, 7*24*time.Hour, c.UntrustedSessionDuration)
				require.True(t, c.CookieSecure)
				require.Empty(t, c.CookieDomain)
				require.Equal(t, "text", c.LogFormat)
				require.Equal(t, "info", c.LogLevel)
				require.Empty(t, c.TrustedProxies, "no proxy trusted by default")
				require.Empty(t, c.AllowedOrigins, "no origin allowed by default")
			},
		},
		{
			name:     "allowed origins read as a list, normalised to lowercase without a trailing slash",
			override: map[string]string{"ALLOWED_ORIGINS": " https://App.Example.com/ ,http://localhost:5173"},
			wantCheck: func(t *testing.T, c Config) {
				require.Equal(t, []string{"https://app.example.com", "http://localhost:5173"}, c.AllowedOrigins)
			},
		},
		{name: "an allowed origin with a path is refused", override: map[string]string{"ALLOWED_ORIGINS": "https://example.com/app"},
			wantErr: `config: ALLOWED_ORIGINS entry "https://example.com/app" is not an origin (scheme://host[:port])`},
		{name: "an allowed origin with a query is refused", override: map[string]string{"ALLOWED_ORIGINS": "https://example.com?x=1"},
			wantErr: `ALLOWED_ORIGINS entry "https://example.com?x=1"`},
		{name: "an allowed origin with an empty query is refused", override: map[string]string{"ALLOWED_ORIGINS": "https://example.com?"},
			wantErr: `ALLOWED_ORIGINS entry "https://example.com?"`},
		{name: "an allowed origin with a fragment is refused", override: map[string]string{"ALLOWED_ORIGINS": "https://example.com#top"},
			wantErr: `ALLOWED_ORIGINS entry "https://example.com#top"`},
		{name: "an allowed origin with user info is refused", override: map[string]string{"ALLOWED_ORIGINS": "https://user:pw@example.com"},
			wantErr: `ALLOWED_ORIGINS entry "https://user:pw@example.com"`},
		{name: "an allowed origin that isn't http or https is refused", override: map[string]string{"ALLOWED_ORIGINS": "ftp://example.com"},
			wantErr: `ALLOWED_ORIGINS entry "ftp://example.com"`},
		{name: "an allowed origin without a scheme is refused", override: map[string]string{"ALLOWED_ORIGINS": "example.com"},
			wantErr: `ALLOWED_ORIGINS entry "example.com"`},
		{name: "an allowed origin without a host is refused", override: map[string]string{"ALLOWED_ORIGINS": "https://:8080"},
			wantErr: `ALLOWED_ORIGINS entry "https://:8080"`},
		{name: "a wildcard origin is refused", override: map[string]string{"ALLOWED_ORIGINS": "*"},
			wantErr: `ALLOWED_ORIGINS entry "*"`},
		{name: "an allowed origin that doesn't parse is refused", override: map[string]string{"ALLOWED_ORIGINS": "https://exa mple.com"},
			wantErr: `ALLOWED_ORIGINS entry "https://exa mple.com"`},
		{
			name:     "trusted proxies read as a trimmed list of IPs and CIDRs",
			override: map[string]string{"TRUSTED_PROXIES": " 10.0.0.1, 192.168.0.0/16 ,,2001:db8::1"},
			wantCheck: func(t *testing.T, c Config) {
				require.Equal(t, []string{"10.0.0.1", "192.168.0.0/16", "2001:db8::1"}, c.TrustedProxies)
			},
		},
		{name: "a trusted proxy that is neither an IP nor a CIDR", override: map[string]string{"TRUSTED_PROXIES": "10.0.0.1,proxy.local"},
			wantErr: `TRUSTED_PROXIES: "proxy.local" is not an IP or a CIDR`},
		{name: "a trusted proxy CIDR with a bad mask", override: map[string]string{"TRUSTED_PROXIES": "10.0.0.0/33"},
			wantErr: `TRUSTED_PROXIES: "10.0.0.0/33"`},
		{
			name:     "log format and level read",
			override: map[string]string{"LOG_FORMAT": "json", "LOG_LEVEL": "debug"},
			wantCheck: func(t *testing.T, c Config) {
				require.Equal(t, "json", c.LogFormat)
				require.Equal(t, "debug", c.LogLevel)
			},
		},
		{name: "an invalid log format", override: map[string]string{"LOG_FORMAT": "xml"}, wantErr: "LOG_FORMAT"},
		{name: "an invalid log level", override: map[string]string{"LOG_LEVEL": "loud"}, wantErr: `log level "loud"`},
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
			name:     "an untrusted session duration read",
			override: map[string]string{"UNTRUSTED_SESSION_DURATION": "24h"},
			wantCheck: func(t *testing.T, c Config) {
				require.Equal(t, 24*time.Hour, c.UntrustedSessionDuration)
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
		{name: "an invalid untrusted session duration", override: map[string]string{"UNTRUSTED_SESSION_DURATION": "never"}, wantErr: "UNTRUSTED_SESSION_DURATION"},
		{name: "an invalid bool", override: map[string]string{"COOKIE_SECURE": "maybe"}, wantErr: "COOKIE_SECURE"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range validEnv {
				t.Setenv(k, v)
			}
			t.Setenv("UNTRUSTED_SESSION_DURATION", "")
			t.Setenv("COOKIE_SECURE", "")
			t.Setenv("COOKIE_DOMAIN", "")
			t.Setenv("LOG_FORMAT", "")
			t.Setenv("LOG_LEVEL", "")
			t.Setenv("TRUSTED_PROXIES", "")
			t.Setenv("ALLOWED_ORIGINS", "")
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

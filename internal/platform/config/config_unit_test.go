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
	"APP_BASE_URL":           "https://app.example.com",
	"SMTP_HOST":              "smtp.example.com",
	"SMTP_FROM":              "App <no-reply@example.com>",
}

func TestLoadConfig_Env_U(t *testing.T) {
	testCases := []struct {
		name      string
		override  map[string]string
		wantErr   string
		wantNot   string // must not appear in the error
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
				require.Empty(t, c.APIKeys, "no API key by default")
				require.Equal(t, 587, c.SMTPPort, "SMTP submission port by default")
				require.Equal(t, "https://app.example.com", c.AppBaseURL)
				require.Equal(t, 17520*time.Hour, c.ActivityLogRetention, "the activity log is kept 2 years by default")
			},
		},
		{
			name:     "an activity log retention read",
			override: map[string]string{"ACTIVITY_LOG_RETENTION": "720h"},
			wantCheck: func(t *testing.T, c Config) {
				require.Equal(t, 720*time.Hour, c.ActivityLogRetention)
			},
		},
		{
			name:     "an activity log retention of 0 keeps the log forever",
			override: map[string]string{"ACTIVITY_LOG_RETENTION": "0"},
			wantCheck: func(t *testing.T, c Config) {
				require.Zero(t, c.ActivityLogRetention)
			},
		},
		{
			name:     "telegram settings read as a pair",
			override: map[string]string{"TELEGRAM_BOT_TOKEN": "123:abc", "TELEGRAM_CHAT_ID": "-100"},
			wantCheck: func(t *testing.T, c Config) {
				require.Equal(t, "123:abc", c.TelegramBotToken)
				require.Equal(t, "-100", c.TelegramChatID)
			},
		},
		{name: "a telegram token without a chat", override: map[string]string{"TELEGRAM_BOT_TOKEN": "123:abc"},
			wantErr: "config: TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID go together"},
		{name: "a telegram chat without a token", override: map[string]string{"TELEGRAM_CHAT_ID": "-100"},
			wantErr: "config: TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID go together"},
		{
			name:     "a Redis URL read",
			override: map[string]string{"REDIS_URL": "rediss://:s3cret@redis.example.com:6380/0"},
			wantCheck: func(t *testing.T, c Config) {
				require.Equal(t, "rediss://:s3cret@redis.example.com:6380/0", c.RedisURL)
			},
		},
		{name: "a Redis URL with another scheme is refused", override: map[string]string{"REDIS_URL": "http://redis.example.com"},
			wantErr: "config: invalid REDIS_URL: redis: invalid URL scheme: http"},
		{name: "a Redis URL that doesn't parse is refused without echoing it (it can hold a password)",
			override: map[string]string{"REDIS_URL": "redis://:pa%zzss@host"}, wantErr: "config: invalid REDIS_URL",
			wantNot: "pa%zzss"},
		{
			name:     "API keys read as name to key",
			override: map[string]string{"API_KEYS": " billing:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb , reporting:rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr"},
			wantCheck: func(t *testing.T, c Config) {
				require.Equal(t, map[string]string{"billing": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "reporting": "rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr"}, c.APIKeys)
			},
		},
		{name: "an API key entry without a name is refused", override: map[string]string{"API_KEYS": ":bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
			wantErr: "config: API_KEYS entry 1 is not name:key", wantNot: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		{name: "an API key entry without a colon is refused", override: map[string]string{"API_KEYS": "billing"},
			wantErr: "config: API_KEYS entry 1 is not name:key"},
		{name: "a short API key is refused without echoing it", override: map[string]string{"API_KEYS": "billing:shortsecret"},
			wantErr: "config: API_KEYS entry 1 has a key shorter than 32 characters", wantNot: "shortsecret"},
		{name: "an API client name used twice is refused, naming the entries", override: map[string]string{"API_KEYS": "billing:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb,billing:rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr"},
			wantErr: "config: API_KEYS entry 2 repeats the name of entry 1", wantNot: "rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr"},
		{name: "a key written before its name (key:name): the error doesn't echo the key",
			override: map[string]string{"API_KEYS": "kkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkk:billing"},
			wantErr:  "config: API_KEYS entry 1 has a key shorter than 32 characters", wantNot: "kkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkk"},
		{name: "the same key written before two names: the error doesn't echo the key",
			override: map[string]string{"API_KEYS": "kkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkk:billing-client-name-of-32-chars-x,kkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkk:reporting-client-name-of-32-chars"},
			wantErr:  "config: API_KEYS entry 2 repeats the name of entry 1", wantNot: "kkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkkk"},
		{name: "a negative activity log retention", override: map[string]string{"ACTIVITY_LOG_RETENTION": "-1h"}, wantErr: "ACTIVITY_LOG_RETENTION -1h0m0s is negative"},
		{name: "an activity log retention that isn't a duration", override: map[string]string{"ACTIVITY_LOG_RETENTION": "forever"}, wantErr: "ACTIVITY_LOG_RETENTION"},
		{
			name: "SMTP settings read",
			override: map[string]string{"SMTP_PORT": "465", "SMTP_USERNAME": "mailer", "SMTP_PASSWORD": "secret",
				"SMTP_FROM": "no-reply@example.com"},
			wantCheck: func(t *testing.T, c Config) {
				require.Equal(t, "smtp.example.com", c.SMTPHost)
				require.Equal(t, 465, c.SMTPPort)
				require.Equal(t, "mailer", c.SMTPUsername)
				require.Equal(t, "secret", c.SMTPPassword)
				require.Equal(t, "no-reply@example.com", c.SMTPFrom)
			},
		},
		{
			name:     "in development the SMTP host and sender may be empty (emails are logged)",
			override: map[string]string{"ENVIRONMENT": "development", "SMTP_HOST": "", "SMTP_FROM": ""},
			wantCheck: func(t *testing.T, c Config) {
				require.Empty(t, c.SMTPHost)
			},
		},
		{name: "outside development the SMTP host is required", override: map[string]string{"SMTP_HOST": ""}, wantErr: "SMTP_HOST is required"},
		{name: "outside development the sender is required", override: map[string]string{"SMTP_FROM": ""}, wantErr: "SMTP_FROM is required"},
		{name: "in development with an SMTP host (a local mail sink) the sender is required: every send would fail without it",
			override: map[string]string{"ENVIRONMENT": "development", "SMTP_HOST": "localhost", "SMTP_FROM": ""},
			wantErr:  "SMTP_FROM is required"},
		{name: "a sender that isn't an address", override: map[string]string{"SMTP_FROM": "nobody"}, wantErr: `SMTP_FROM "nobody" is not an email address`},
		{name: "an SMTP port that isn't a number", override: map[string]string{"SMTP_PORT": "smtp"}, wantErr: "SMTP_PORT"},
		{name: "an SMTP port out of range", override: map[string]string{"SMTP_PORT": "70000"}, wantErr: "SMTP_PORT 70000 is not a port"},
		{name: "the app base URL is required", override: map[string]string{"APP_BASE_URL": ""}, wantErr: "APP_BASE_URL is required"},
		{name: "an app base URL with a trailing slash", override: map[string]string{"APP_BASE_URL": "https://app.example.com/"},
			wantErr: `APP_BASE_URL "https://app.example.com/" is not an http(s) URL`},
		{name: "an app base URL that isn't http(s)", override: map[string]string{"APP_BASE_URL": "ftp://app.example.com"},
			wantErr: "APP_BASE_URL"},
		{name: "an app base URL with a query", override: map[string]string{"APP_BASE_URL": "https://app.example.com?x=1"},
			wantErr: "APP_BASE_URL"},
		{name: "an app base URL that doesn't parse", override: map[string]string{"APP_BASE_URL": "https://exa mple.com"},
			wantErr: "APP_BASE_URL"},
		{
			name:     "an app base URL with a path",
			override: map[string]string{"APP_BASE_URL": "https://example.com/app"},
			wantCheck: func(t *testing.T, c Config) {
				require.Equal(t, "https://example.com/app", c.AppBaseURL)
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
		{
			name:     "a hybrid app's own origin (Capacitor / Ionic WebView on iOS) is accepted, normalised",
			override: map[string]string{"ALLOWED_ORIGINS": "https://app.example.com,Capacitor://Localhost,ionic://localhost/"},
			wantCheck: func(t *testing.T, c Config) {
				require.Equal(t, []string{"https://app.example.com", "capacitor://localhost", "ionic://localhost"}, c.AllowedOrigins)
			},
		},
		{name: "a browser-internal scheme is refused (file:// pages send Origin null)", override: map[string]string{"ALLOWED_ORIGINS": "file://localhost"},
			wantErr: `ALLOWED_ORIGINS entry "file://localhost"`},
		{name: "the literal null origin is refused (sandboxed frames, file:// pages)", override: map[string]string{"ALLOWED_ORIGINS": "null"},
			wantErr: `ALLOWED_ORIGINS entry "null"`},
		{name: "an app scheme still needs a host", override: map[string]string{"ALLOWED_ORIGINS": "capacitor://"},
			wantErr: `ALLOWED_ORIGINS entry "capacitor://"`},
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
			t.Setenv("SMTP_PORT", "")
			t.Setenv("ACTIVITY_LOG_RETENTION", "")
			t.Setenv("TELEGRAM_BOT_TOKEN", "")
			t.Setenv("TELEGRAM_CHAT_ID", "")
			t.Setenv("REDIS_URL", "")
			t.Setenv("API_KEYS", "")
			t.Setenv("SMTP_USERNAME", "")
			t.Setenv("SMTP_PASSWORD", "")
			for k, v := range tc.override {
				t.Setenv(k, v)
			}

			c, err := LoadConfig(t.TempDir())
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				if tc.wantNot != "" {
					require.NotContains(t, err.Error(), tc.wantNot)
				}
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

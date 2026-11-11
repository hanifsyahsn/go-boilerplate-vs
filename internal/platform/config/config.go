package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

type Config struct {
	Environment         string        `mapstructure:"ENVIRONMENT"`
	ServerAddress       string        `mapstructure:"SERVER_ADDRESS"`
	DBDriver            string        `mapstructure:"DB_DRIVER"`
	DBSource            string        `mapstructure:"DB_SOURCE"`
	AccessTokenDuration time.Duration `mapstructure:"ACCESS_TOKEN_DURATION"`
	// RefreshTokenDuration is a refresh token's lifetime. Refresh tokens
	// rotate, so it's the idle timeout: a user who doesn't come back within
	// it logs in again. Optional, defaults to DefaultRefreshTokenDuration.
	RefreshTokenDuration time.Duration `mapstructure:"REFRESH_TOKEN_DURATION"`
	// UntrustedSessionDuration caps a login from an untrusted device (one
	// without trust_device): its refresh tokens still rotate, but none lives
	// past login + this. A trusted device's session slides with no cap.
	// Optional, defaults to DefaultUntrustedSessionDuration.
	UntrustedSessionDuration time.Duration `mapstructure:"UNTRUSTED_SESSION_DURATION"`
	TokenIssuer              string        `mapstructure:"TOKEN_ISSUER"`
	// ClientID is the access token's audience (`aud`), checked on verify.
	ClientID         string `mapstructure:"CLIENT_ID"`
	ECPrivateKeyPath string `mapstructure:"EC_PRIVATE_KEY_PATH"`
	ECPublicKeyPath  string `mapstructure:"EC_PUBLIC_KEY_PATH"`
	// CookieSecure marks the auth cookies Secure (HTTPS only). Optional,
	// defaults to true; set false for plain-HTTP local development.
	CookieSecure bool `mapstructure:"COOKIE_SECURE"`
	// CookieDomain is the auth cookies' Domain attribute. Optional — empty
	// means a host-only cookie.
	CookieDomain string `mapstructure:"COOKIE_DOMAIN"`
	// LogFormat is text (default, for development) or json (production).
	LogFormat string `mapstructure:"LOG_FORMAT"`
	// LogLevel is the minimum level logged: debug, info (default), warn or
	// error.
	LogLevel string `mapstructure:"LOG_LEVEL"`
	// TrustedProxies are the reverse proxies (IPs or CIDRs) whose
	// X-Forwarded-For / X-Real-IP gin believes when it works out the client's
	// IP (the per-IP rate limit, the access log). Optional, comma separated;
	// empty trusts no proxy, so the client IP is the connection's address.
	// Behind a proxy, list it — otherwise every client shares its IP.
	TrustedProxies []string `mapstructure:"TRUSTED_PROXIES"`
	// AllowedOrigins (ALLOWED_ORIGINS) are the browser origins allowed to call
	// the API with credentials: one list for CORS and the origin guard.
	// Optional, comma separated; each entry is scheme://host[:port], stored
	// normalised (lowercase, no trailing /). Empty turns both off.
	AllowedOrigins []string `mapstructure:"ALLOWED_ORIGINS"`
	// SMTPHost / SMTPPort / SMTPUsername / SMTPPassword / SMTPFrom are the
	// outgoing mail server (password reset, email verification). Port 465 is
	// implicit TLS, any other requires STARTTLS. SMTP_HOST and SMTP_FROM (an
	// address) are required except in development, where an empty SMTP_HOST
	// logs emails instead of sending them. SMTP_PORT defaults to 587.
	SMTPHost     string `mapstructure:"SMTP_HOST"`
	SMTPPort     int    `mapstructure:"SMTP_PORT"`
	SMTPUsername string `mapstructure:"SMTP_USERNAME"`
	SMTPPassword string `mapstructure:"SMTP_PASSWORD"`
	SMTPFrom     string `mapstructure:"SMTP_FROM"`
	// AppBaseURL is the front end's base URL (http(s)://host[/path], no
	// trailing /); the links in emails are built on it. Required.
	AppBaseURL string `mapstructure:"APP_BASE_URL"`
	// ActivityLogRetention is how long api_activity_logs rows are kept (the
	// cleanup worker deletes older ones). Optional, defaults to
	// DefaultActivityLogRetention; 0 keeps them forever.
	ActivityLogRetention time.Duration `mapstructure:"ACTIVITY_LOG_RETENTION"`
	// TelegramBotToken / TelegramChatID send ops alerts (panics, failed
	// background jobs) to a Telegram chat. Optional, both or neither; neither
	// turns alerts off. The token is a secret: never logged.
	TelegramBotToken string `mapstructure:"TELEGRAM_BOT_TOKEN"`
	TelegramChatID   string `mapstructure:"TELEGRAM_CHAT_ID"`
	// RedisURL (redis:// or rediss://, Redis ≥ 7) moves the rate limits to
	// Redis so every replica counts together. Optional: empty keeps them in
	// memory (one instance). A secret when it carries a password: never
	// logged.
	RedisURL string `mapstructure:"REDIS_URL"`
	// APIKeys (API_KEYS, "name:key,name:key") are the machine callers' keys
	// for middleware.APIKey, name → key. Optional; each key at least
	// MinAPIKeyLength characters, names unique. Secrets: never logged.
	APIKeys map[string]string `mapstructure:"API_KEYS"`
}

// MinAPIKeyLength is the shortest API key accepted.
const MinAPIKeyLength = 32

// DefaultActivityLogRetention is ACTIVITY_LOG_RETENTION when unset: 2 years.
const DefaultActivityLogRetention = 17520 * time.Hour

// DefaultSMTPPort is SMTP_PORT when unset: submission with STARTTLS.
const DefaultSMTPPort = 587

// DefaultRefreshTokenDuration is REFRESH_TOKEN_DURATION when unset: 7 days.
const DefaultRefreshTokenDuration = 7 * 24 * time.Hour

// DefaultUntrustedSessionDuration is UNTRUSTED_SESSION_DURATION when unset: 7
// days.
const DefaultUntrustedSessionDuration = 7 * 24 * time.Hour

// LOG_FORMAT / LOG_LEVEL when unset.
const (
	DefaultLogFormat = logging.FormatText
	DefaultLogLevel  = "info"
)

// LoadConfig reads configuration from a .env file at path (if present) and
// falls back to OS environment variables. A missing .env file is not an
// error; an invalid value for a typed field (duration/bool), or a file that
// exists but can't be read/parsed, is.
func LoadConfig(path string) (Config, error) {
	v := viper.New()

	envFile := filepath.Join(path, ".env")
	if _, statErr := os.Stat(envFile); statErr == nil {
		v.SetConfigFile(envFile)
		v.SetConfigType("env")

		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("config.LoadConfig() (ReadInConfig): %w", err)
		}
	} else {
		slog.Warn("no .env file found, using environment variables only")
	}

	v.AutomaticEnv()

	accessTokenDuration, err := getDurationEnv(v, "ACCESS_TOKEN_DURATION")
	if err != nil {
		return Config{}, err
	}

	refreshTokenDuration, err := getDurationEnv(v, "REFRESH_TOKEN_DURATION")
	if err != nil {
		return Config{}, err
	}
	if refreshTokenDuration == 0 {
		refreshTokenDuration = DefaultRefreshTokenDuration
	}

	untrustedSessionDuration, err := getDurationEnv(v, "UNTRUSTED_SESSION_DURATION")
	if err != nil {
		return Config{}, err
	}
	if untrustedSessionDuration == 0 {
		untrustedSessionDuration = DefaultUntrustedSessionDuration
	}

	cookieSecure, err := getBoolEnv(v, "COOKIE_SECURE", true)
	if err != nil {
		return Config{}, err
	}

	smtpPort, err := getIntEnv(v, "SMTP_PORT", DefaultSMTPPort)
	if err != nil {
		return Config{}, err
	}

	apiKeys, err := parseAPIKeys(getListEnv(v, "API_KEYS"))
	if err != nil {
		return Config{}, err
	}

	// Unset is the default; an explicit 0 keeps the log forever.
	activityLogRetention := DefaultActivityLogRetention
	if v.GetString("ACTIVITY_LOG_RETENTION") != "" {
		if activityLogRetention, err = getDurationEnv(v, "ACTIVITY_LOG_RETENTION"); err != nil {
			return Config{}, err
		}
	}

	config := Config{
		Environment:              v.GetString("ENVIRONMENT"),
		ServerAddress:            v.GetString("SERVER_ADDRESS"),
		DBDriver:                 v.GetString("DB_DRIVER"),
		DBSource:                 v.GetString("DB_SOURCE"),
		AccessTokenDuration:      accessTokenDuration,
		RefreshTokenDuration:     refreshTokenDuration,
		UntrustedSessionDuration: untrustedSessionDuration,
		TokenIssuer:              v.GetString("TOKEN_ISSUER"),
		ClientID:                 v.GetString("CLIENT_ID"),
		ECPrivateKeyPath:         v.GetString("EC_PRIVATE_KEY_PATH"),
		ECPublicKeyPath:          v.GetString("EC_PUBLIC_KEY_PATH"),
		CookieSecure:             cookieSecure,
		CookieDomain:             v.GetString("COOKIE_DOMAIN"),
		LogFormat:                getStringEnv(v, "LOG_FORMAT", DefaultLogFormat),
		LogLevel:                 getStringEnv(v, "LOG_LEVEL", DefaultLogLevel),
		TrustedProxies:           getListEnv(v, "TRUSTED_PROXIES"),
		AllowedOrigins:           getListEnv(v, "ALLOWED_ORIGINS"),
		SMTPHost:                 v.GetString("SMTP_HOST"),
		SMTPPort:                 smtpPort,
		SMTPUsername:             v.GetString("SMTP_USERNAME"),
		SMTPPassword:             v.GetString("SMTP_PASSWORD"),
		SMTPFrom:                 v.GetString("SMTP_FROM"),
		AppBaseURL:               v.GetString("APP_BASE_URL"),
		ActivityLogRetention:     activityLogRetention,
		TelegramBotToken:         v.GetString("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:           v.GetString("TELEGRAM_CHAT_ID"),
		RedisURL:                 v.GetString("REDIS_URL"),
		APIKeys:                  apiKeys,
	}

	if err = config.Validate(); err != nil {
		return Config{}, err
	}
	for i, origin := range config.AllowedOrigins {
		config.AllowedOrigins[i], _ = NormalizeOrigin(origin) // Validate checked every entry
	}

	return config, nil
}

func getStringEnv(v *viper.Viper, key, defaultVal string) string {
	if val := v.GetString(key); val != "" {
		return val
	}
	return defaultVal
}

// getListEnv reads a comma-separated env var, trimming each entry and dropping
// empty ones; unset is nil.
func getListEnv(v *viper.Viper, key string) []string {
	var out []string
	for _, item := range strings.Split(v.GetString(key), ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func getDurationEnv(v *viper.Viper, key string) (time.Duration, error) {
	val := v.GetString(key)
	if val == "" {
		return 0, nil
	}

	d, err := time.ParseDuration(val)
	if err != nil {
		return 0, fmt.Errorf("config: invalid value for env %s: %q: %w", key, val, err)
	}

	return d, nil
}

// getIntEnv reads an integer env var, returning defaultVal when unset.
func getIntEnv(v *viper.Viper, key string, defaultVal int) (int, error) {
	val := v.GetString(key)
	if val == "" {
		return defaultVal, nil
	}

	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("config: invalid value for env %s: %q: %w", key, val, err)
	}

	return n, nil
}

// getBoolEnv reads a boolean env var, returning defaultVal when unset (a
// bool's zero value, false, isn't always the wanted default).
func getBoolEnv(v *viper.Viper, key string, defaultVal bool) (bool, error) {
	val := v.GetString(key)
	if val == "" {
		return defaultVal, nil
	}

	b, err := strconv.ParseBool(val)
	if err != nil {
		return false, fmt.Errorf("config: invalid value for env %s: %q: %w", key, val, err)
	}

	return b, nil
}

// Validate enforces every required field. Optional knobs
// (REFRESH_TOKEN_DURATION, UNTRUSTED_SESSION_DURATION, COOKIE_*, LOG_*, TRUSTED_PROXIES,
// ALLOWED_ORIGINS) get their defaults in LoadConfig, which also normalises
// ALLOWED_ORIGINS after this check;
// LOG_FORMAT / LOG_LEVEL must still be values logging accepts, and each
// TRUSTED_PROXIES entry an IP or a CIDR.
func (c Config) Validate() error {
	required := []struct {
		key   string
		empty bool
	}{
		{"ENVIRONMENT", c.Environment == ""},
		{"SERVER_ADDRESS", c.ServerAddress == ""},
		{"DB_DRIVER", c.DBDriver == ""},
		{"DB_SOURCE", c.DBSource == ""},
		{"ACCESS_TOKEN_DURATION", c.AccessTokenDuration == 0},
		{"TOKEN_ISSUER", c.TokenIssuer == ""},
		{"CLIENT_ID", c.ClientID == ""},
		{"EC_PRIVATE_KEY_PATH", c.ECPrivateKeyPath == ""},
		{"EC_PUBLIC_KEY_PATH", c.ECPublicKeyPath == ""},
		{"APP_BASE_URL", c.AppBaseURL == ""},
		{"SMTP_HOST", c.SMTPHost == "" && c.Environment != EnvironmentDevelopment},
		{"SMTP_FROM", c.SMTPFrom == "" && c.Environment != EnvironmentDevelopment},
	}

	var errs []error
	for _, r := range required {
		if r.empty {
			errs = append(errs, fmt.Errorf("%s is required", r.key))
		}
	}
	if err := logging.Check(c.LogFormat, c.LogLevel); err != nil {
		errs = append(errs, fmt.Errorf("LOG_FORMAT / LOG_LEVEL: %w", err))
	}
	for _, proxy := range c.TrustedProxies {
		if !validProxy(proxy) {
			errs = append(errs, fmt.Errorf("TRUSTED_PROXIES: %q is not an IP or a CIDR", proxy))
		}
	}
	if c.AppBaseURL != "" && !validBaseURL(c.AppBaseURL) {
		errs = append(errs, fmt.Errorf("config: APP_BASE_URL %q is not an http(s) URL without a trailing / (https://app.example.com)", c.AppBaseURL))
	}
	if c.SMTPFrom != "" {
		if _, err := mail.ParseAddress(c.SMTPFrom); err != nil {
			errs = append(errs, fmt.Errorf("config: SMTP_FROM %q is not an email address: %w", c.SMTPFrom, err))
		}
	}
	if c.RedisURL != "" {
		if _, err := redis.ParseURL(c.RedisURL); err != nil {
			errs = append(errs, fmt.Errorf("config: invalid REDIS_URL: %s", redactURLError(err)))
		}
	}
	if (c.TelegramBotToken == "") != (c.TelegramChatID == "") {
		errs = append(errs, errors.New("config: TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID go together"))
	}
	if c.ActivityLogRetention < 0 {
		errs = append(errs, fmt.Errorf("config: ACTIVITY_LOG_RETENTION %s is negative (0 keeps the log forever)", c.ActivityLogRetention))
	}
	if c.SMTPPort < 1 || c.SMTPPort > 65535 {
		errs = append(errs, fmt.Errorf("config: SMTP_PORT %d is not a port", c.SMTPPort))
	}
	for _, origin := range c.AllowedOrigins {
		if _, err := NormalizeOrigin(origin); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// NormalizeOrigin checks that s is a browser origin, scheme://host[:port]
// with scheme http or https and nothing after the host but an optional /, and
// returns it lowercased without the trailing /. "*" is refused: the API sends
// credentials, which a wildcard origin can't carry.
func NormalizeOrigin(s string) (string, error) {
	notOrigin := fmt.Errorf("config: ALLOWED_ORIGINS entry %q is not an origin (scheme://host[:port])", s)
	u, err := url.Parse(s)
	if err != nil {
		return "", notOrigin
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme != "http" && scheme != "https") || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		strings.Contains(s, "#") || u.Opaque != "" {
		return "", notOrigin
	}
	return scheme + "://" + strings.ToLower(u.Host), nil
}

// parseAPIKeys reads API_KEYS entries ("name:key"). An error names the entry
// or the client, never a key.
func parseAPIKeys(entries []string) (map[string]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	keys := make(map[string]string, len(entries))
	var errs []error
	for i, entry := range entries {
		name, key, ok := strings.Cut(entry, ":")
		name, key = strings.TrimSpace(name), strings.TrimSpace(key)
		switch {
		case !ok || name == "" || key == "":
			errs = append(errs, fmt.Errorf("config: API_KEYS entry %d is not name:key", i+1))
		case len(key) < MinAPIKeyLength:
			errs = append(errs, fmt.Errorf("config: API_KEYS key of %q is shorter than %d characters", name, MinAPIKeyLength))
		case keys[name] != "":
			errs = append(errs, fmt.Errorf("config: API_KEYS name %q is used twice", name))
		default:
			keys[name] = key
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return keys, nil
}

// redactURLError is err's text without the URL a *url.Error quotes (a
// REDIS_URL can carry a password); other errors are kept as they are.
func redactURLError(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}

// EnvironmentDevelopment is the ENVIRONMENT of a developer's machine: the
// SMTP settings are optional there (emails are logged).
const EnvironmentDevelopment = "development"

// validBaseURL reports whether s is an absolute http(s) URL with a host, no
// query, fragment or user info, and no trailing / (links are s + "/path").
func validBaseURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil &&
		u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && !strings.HasSuffix(s, "/")
}

// validProxy reports whether s is an IP or a CIDR, the forms gin's
// SetTrustedProxies accepts.
func validProxy(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	_, _, err := net.ParseCIDR(s)
	return err == nil
}

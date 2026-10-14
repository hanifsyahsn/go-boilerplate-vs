package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
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
}

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
	}

	if err = config.Validate(); err != nil {
		return Config{}, err
	}

	return config, nil
}

func getStringEnv(v *viper.Viper, key, defaultVal string) string {
	if val := v.GetString(key); val != "" {
		return val
	}
	return defaultVal
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
// (REFRESH_TOKEN_DURATION, UNTRUSTED_SESSION_DURATION, COOKIE_*, LOG_*) get their defaults in LoadConfig;
// LOG_FORMAT / LOG_LEVEL must still be values logging accepts.
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
	return errors.Join(errs...)
}

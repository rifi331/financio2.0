// Package config loads Financio's environment-based configuration.
package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds all runtime configuration read from the environment.
type Config struct {
	TelegramBotToken string
	OpenAIAPIKey     string
	DatabaseURL      string
	OpenAIModel      string
	AllowedUsers     []string
	DisplayTZ        string
	DefaultCurrency  string
	HealthPort       string
	LogLevel         string
	DashboardToken   string // optional; enables the web dashboard when set
}

// Load reads configuration from the environment, applying defaults and
// failing fast with a clear message when a required variable is missing.
func Load() (*Config, error) {
	cfg := &Config{
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		OpenAIAPIKey:     os.Getenv("OPENAI_API_KEY"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		OpenAIModel:      getenvDefault("OPENAI_MODEL", "gpt-6-luna"),
		DisplayTZ:        getenvDefault("DISPLAY_TZ", "Asia/Kuala_Lumpur"),
		DefaultCurrency:  getenvDefault("DEFAULT_CURRENCY", "MYR"),
		HealthPort:       getenvDefault("HEALTH_PORT", "7777"),
		LogLevel:         getenvDefault("LOG_LEVEL", "info"),
		DashboardToken:   strings.TrimSpace(os.Getenv("DASHBOARD_TOKEN")),
	}

	if cfg.TelegramBotToken == "" {
		return nil, fmt.Errorf("config: TELEGRAM_BOT_TOKEN is required but not set")
	}
	if cfg.OpenAIAPIKey == "" {
		return nil, fmt.Errorf("config: OPENAI_API_KEY is required but not set")
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL is required but not set")
	}

	raw := strings.TrimSpace(os.Getenv("ALLOWED_USERS"))
	if raw == "" {
		return nil, fmt.Errorf("config: ALLOWED_USERS is required but not set (comma-separated display names)")
	}
	for _, name := range strings.Split(raw, ",") {
		if name = strings.TrimSpace(name); name != "" {
			cfg.AllowedUsers = append(cfg.AllowedUsers, name)
		}
	}
	if len(cfg.AllowedUsers) == 0 {
		return nil, fmt.Errorf("config: ALLOWED_USERS is required but contained no names")
	}

	return cfg, nil
}

func getenvDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// String returns a human-readable, redacted representation of the config.
// Tokens, API keys, and DSN passwords are never included.
func (c *Config) String() string {
	return fmt.Sprintf(
		"Config{TelegramBotToken: %s, OpenAIAPIKey: %s, DatabaseURL: %s, OpenAIModel: %s, AllowedUsers: %v, DisplayTZ: %s, DefaultCurrency: %s, HealthPort: %s, LogLevel: %s, DashboardToken: %s}",
		redact(c.TelegramBotToken),
		redact(c.OpenAIAPIKey),
		redactDSN(c.DatabaseURL),
		c.OpenAIModel,
		c.AllowedUsers,
		c.DisplayTZ,
		c.DefaultCurrency,
		c.HealthPort,
		c.LogLevel,
		redact(c.DashboardToken),
	)
}

func redact(s string) string {
	if s == "" {
		return "(unset)"
	}
	return "(set)"
}

// redactDSN strips the password from a postgres DSN while keeping host/db
// visible for debugging. Non-URL values are redacted entirely.
func redactDSN(dsn string) string {
	const scheme = "postgres://"
	if !strings.HasPrefix(dsn, scheme) && !strings.HasPrefix(dsn, "postgresql://") {
		return "(set)"
	}
	rest := dsn[strings.Index(dsn, "://")+3:]
	if at := strings.Index(rest, "@"); at >= 0 {
		if colon := strings.Index(rest[:at], ":"); colon >= 0 {
			return dsn[:strings.Index(dsn, "://")+3+colon+1] + "***" + rest[at:]
		}
	}
	return dsn
}

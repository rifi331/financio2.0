package config

import (
	"strings"
	"testing"
)

func TestLoadFailsFastWhenTelegramTokenUnset(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("OPENAI_API_KEY", "k")
	t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/db")
	t.Setenv("ALLOWED_USERS", "Rifi Anggriawan")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when TELEGRAM_BOT_TOKEN is unset, got nil")
	}
	want := "TELEGRAM_BOT_TOKEN"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not mention %q", err.Error(), want)
	}
}

func TestLoadFailsFastWhenOtherRequiredUnset(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "tok")
	t.Setenv("OPENAI_API_KEY", "k")
	t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/db")
	t.Setenv("ALLOWED_USERS", "")

	if _, err := Load(); err == nil {
		t.Error("expected error when ALLOWED_USERS is unset, got nil")
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "tok")
	t.Setenv("OPENAI_API_KEY", "k")
	t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/db")
	t.Setenv("ALLOWED_USERS", "Rifi Anggriawan, Jane Doe")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.OpenAIModel != "gpt-6-luna" || cfg.DisplayTZ != "Asia/Kuala_Lumpur" ||
		cfg.DefaultCurrency != "MYR" || cfg.HealthPort != "7777" || cfg.LogLevel != "info" {
		t.Errorf("defaults not applied: %+v", cfg)
	}
	if len(cfg.AllowedUsers) != 2 || cfg.AllowedUsers[0] != "Rifi Anggriawan" {
		t.Errorf("AllowedUsers parsed wrong: %v", cfg.AllowedUsers)
	}
}

func TestStringRedactsSecrets(t *testing.T) {
	cfg := &Config{
		TelegramBotToken: "123:secret",
		OpenAIAPIKey:     "sk-secret",
		DatabaseURL:      "postgres://user:pass@host:5432/financio",
	}
	s := cfg.String()
	for _, secret := range []string{"123:secret", "sk-secret", ":pass@"} {
		if strings.Contains(s, secret) {
			t.Errorf("String() leaked secret %q: %s", secret, s)
		}
	}
}

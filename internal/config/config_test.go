package config

import (
	"log/slog"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Env != "development" || c.HTTPAddr != ":8080" || c.LogLevel != slog.LevelInfo {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if c.DashboardURL.String() != "http://localhost:8080" {
		t.Fatalf("dashboard URL should default to public URL, got %s", c.DashboardURL)
	}
	if got := c.OAuthRedirectURI(); got != "http://localhost:8080/auth/callback" {
		t.Fatalf("redirect URI = %s", got)
	}
	if c.SecureCookies() {
		t.Fatal("http public URL must not set secure cookies")
	}
}

func TestLoadValues(t *testing.T) {
	c, err := Load(env(map[string]string{
		"NIRO_ENV":                   "production",
		"NIRO_LOG_LEVEL":             "debug",
		"NIRO_PUBLIC_URL":            "https://app.example.gg/",
		"NIRO_DISCORD_CLIENT_ID":     "1234567890123456789",
		"NIRO_DISCORD_CLIENT_SECRET": "s",
		"NIRO_DISCORD_TOKEN":         "t",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.LogLevel != slog.LevelDebug || !c.SecureCookies() || c.Discord.ClientID != 1234567890123456789 {
		t.Fatalf("unexpected config: %+v", c)
	}
	if got := c.OAuthRedirectURI(); got != "https://app.example.gg/auth/callback" {
		t.Fatalf("trailing slash not normalised: %s", got)
	}
	if err := c.Validate(Role{Bot: true, API: true}); err != nil {
		t.Fatalf("valid production config rejected: %v", err)
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	_, err := Load(env(map[string]string{
		"NIRO_ENV":               "staging",
		"NIRO_LOG_FORMAT":        "xml",
		"NIRO_PUBLIC_URL":        "localhost:8080",
		"NIRO_DISCORD_CLIENT_ID": "abc",
	}))
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{"NIRO_ENV", "NIRO_LOG_FORMAT", "NIRO_PUBLIC_URL", "NIRO_DISCORD_CLIENT_ID"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s: %v", want, err)
		}
	}
}

func TestValidateByRole(t *testing.T) {
	c, _ := Load(env(nil))
	if err := c.Validate(Role{}); err != nil {
		t.Fatalf("no role should need nothing: %v", err)
	}
	if err := c.Validate(Role{Bot: true}); err == nil || !strings.Contains(err.Error(), "NIRO_DISCORD_TOKEN") {
		t.Fatalf("bot without token: %v", err)
	}
	err := c.Validate(Role{API: true})
	if err == nil || !strings.Contains(err.Error(), "CLIENT_ID") || !strings.Contains(err.Error(), "CLIENT_SECRET") {
		t.Fatalf("api without oauth credentials: %v", err)
	}

	prod, _ := Load(env(map[string]string{
		"NIRO_ENV": "production", "NIRO_DISCORD_TOKEN": "t", "NIRO_DEV_GUILD_ID": "1",
		"NIRO_DISCORD_CLIENT_ID": "1", "NIRO_DISCORD_CLIENT_SECRET": "s",
	}))
	err = prod.Validate(Role{Bot: true, API: true})
	if err == nil || !strings.Contains(err.Error(), "https") || !strings.Contains(err.Error(), "DEV_GUILD_ID") {
		t.Fatalf("production safety checks missing: %v", err)
	}
}

func TestTrustedProxies(t *testing.T) {
	c, err := Load(env(map[string]string{"NIRO_TRUSTED_PROXIES": "10.0.0.0/8, 172.18.0.5 ,::1"}))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range c.TrustedProxies {
		got = append(got, p.String())
	}
	if strings.Join(got, " ") != "10.0.0.0/8 172.18.0.5/32 ::1/128" {
		t.Fatalf("trusted proxies = %v", got)
	}
	if _, err := Load(env(map[string]string{"NIRO_TRUSTED_PROXIES": "proxy.local"})); err == nil || !strings.Contains(err.Error(), "NIRO_TRUSTED_PROXIES") {
		t.Fatalf("a host name should be rejected: %v", err)
	}
}

func TestProductionNeedsHTTPS(t *testing.T) {
	c, _ := Load(env(map[string]string{
		"NIRO_ENV": "production", "NIRO_PUBLIC_URL": "http://niro.example",
		"NIRO_DISCORD_CLIENT_ID": "1", "NIRO_DISCORD_CLIENT_SECRET": "s",
	}))
	if err := c.Validate(Role{API: true}); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("plain http in production: %v", err)
	}
}

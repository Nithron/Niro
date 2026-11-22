// Package config loads Nithro's runtime configuration from environment
// variables (optionally seeded from a .env file in development).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/disgoorg/snowflake/v2"
)

// Config is the full runtime configuration. Which fields are required depends
// on the role a process runs (bot, api or both); see Validate.
type Config struct {
	Env       string // "development" or "production"
	LogLevel  slog.Level
	LogFormat string // "text" or "json"

	HTTPAddr string // dashboard + API
	OpsAddr  string // health checks and Prometheus metrics

	// PublicURL is where the API/dashboard is reachable from the browser.
	// It determines the OAuth2 redirect URI and whether cookies are Secure.
	PublicURL *url.URL
	// DashboardURL is where users land after login. Defaults to PublicURL;
	// set it to the Vite dev server during dashboard development.
	DashboardURL *url.URL

	DatabaseURL    string
	ValkeyAddr     string
	ValkeyPassword string

	Discord Discord
}

type Discord struct {
	Token        string
	ClientID     snowflake.ID
	ClientSecret string
	// DevGuildID, when set, registers slash commands to that guild only, which
	// applies instantly. Leave empty in production to register globally.
	DevGuildID snowflake.ID
}

// Role is what a process runs.
type Role struct{ Bot, API bool }

func (c Config) IsProduction() bool { return c.Env == "production" }

// SecureCookies reports whether session cookies must carry the Secure flag.
func (c Config) SecureCookies() bool { return c.PublicURL.Scheme == "https" }

// OAuthRedirectURI is the callback URL registered in the Developer Portal.
func (c Config) OAuthRedirectURI() string {
	return strings.TrimSuffix(c.PublicURL.String(), "/") + "/auth/callback"
}

// Load reads configuration through getenv (os.Getenv in production code).
func Load(getenv func(string) string) (Config, error) {
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}
	var errs []error

	c := Config{
		Env:            get("NITHRO_ENV", "development"),
		LogFormat:      get("NITHRO_LOG_FORMAT", "text"),
		HTTPAddr:       get("NITHRO_HTTP_ADDR", ":8080"),
		OpsAddr:        get("NITHRO_OPS_ADDR", ":9090"),
		DatabaseURL:    get("NITHRO_DATABASE_URL", "postgres://nithro:nithro@localhost:5432/nithro?sslmode=disable"),
		ValkeyAddr:     get("NITHRO_VALKEY_ADDR", "localhost:6379"),
		ValkeyPassword: get("NITHRO_VALKEY_PASSWORD", ""),
		Discord: Discord{
			Token:        get("NITHRO_DISCORD_TOKEN", ""),
			ClientSecret: get("NITHRO_DISCORD_CLIENT_SECRET", ""),
		},
	}
	if c.Env != "development" && c.Env != "production" {
		errs = append(errs, fmt.Errorf("NITHRO_ENV must be development or production, got %q", c.Env))
	}
	if c.LogFormat != "text" && c.LogFormat != "json" {
		errs = append(errs, fmt.Errorf("NITHRO_LOG_FORMAT must be text or json, got %q", c.LogFormat))
	}
	if err := c.LogLevel.UnmarshalText([]byte(get("NITHRO_LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("NITHRO_LOG_LEVEL: %w", err))
	}

	var err error
	if c.PublicURL, err = parseBaseURL(get("NITHRO_PUBLIC_URL", "http://localhost:8080")); err != nil {
		errs = append(errs, fmt.Errorf("NITHRO_PUBLIC_URL: %w", err))
	}
	if v := get("NITHRO_DASHBOARD_URL", ""); v != "" {
		if c.DashboardURL, err = parseBaseURL(v); err != nil {
			errs = append(errs, fmt.Errorf("NITHRO_DASHBOARD_URL: %w", err))
		}
	} else {
		c.DashboardURL = c.PublicURL
	}

	if c.Discord.ClientID, err = optionalSnowflake(get("NITHRO_DISCORD_CLIENT_ID", "")); err != nil {
		errs = append(errs, fmt.Errorf("NITHRO_DISCORD_CLIENT_ID: %w", err))
	}
	if c.Discord.DevGuildID, err = optionalSnowflake(get("NITHRO_DEV_GUILD_ID", "")); err != nil {
		errs = append(errs, fmt.Errorf("NITHRO_DEV_GUILD_ID: %w", err))
	}
	return c, errors.Join(errs...)
}

// Validate checks that everything the given role needs is present.
func (c Config) Validate(role Role) error {
	var errs []error
	need := func(ok bool, key string) {
		if !ok {
			errs = append(errs, fmt.Errorf("%s is required", key))
		}
	}
	if role.Bot {
		need(c.Discord.Token != "", "NITHRO_DISCORD_TOKEN")
	}
	if role.API {
		need(c.Discord.ClientID != 0, "NITHRO_DISCORD_CLIENT_ID")
		need(c.Discord.ClientSecret != "", "NITHRO_DISCORD_CLIENT_SECRET")
		if c.IsProduction() && !c.SecureCookies() {
			errs = append(errs, errors.New("NITHRO_PUBLIC_URL must use https in production"))
		}
	}
	if role.Bot && c.IsProduction() && c.Discord.DevGuildID != 0 {
		errs = append(errs, errors.New("NITHRO_DEV_GUILD_ID must not be set in production"))
	}
	return errors.Join(errs...)
}

func parseBaseURL(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("must be an absolute http(s) URL, got %q", s)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("must not contain a query or fragment, got %q", s)
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u, nil
}

func optionalSnowflake(s string) (snowflake.ID, error) {
	if s == "" {
		return 0, nil
	}
	return snowflake.Parse(s)
}

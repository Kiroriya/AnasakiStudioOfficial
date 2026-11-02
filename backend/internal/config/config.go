// Package config loads runtime settings from environment variables.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// Product is shown by the setup wizard, the public site endpoint and the server log.
const Product = "Anasaki Studio"

// Version is stamped at build time from the VERSION file at the repository root
// (the Dockerfile reads it; -ldflags "-X github.com/anasaki/studio/backend/internal/config.Version=2.0.0").
// "dev" = a build without a stamp (go run, tests).
var Version = "dev"

type Config struct {
	Addr          string // HTTP listen address
	DSN           string // MySQL / MariaDB DSN (go-sql-driver format)
	JWTSecret     string
	UploadDir     string // where media files are stored
	AdminDir      string // built Angular app (served at /<admin_path>/)
	AdminEmail    string // when set together with AdminPassword, the admin is created on first run (no setup wizard)
	AdminPassword string
	CORSOrigin    string // extra origin allowed to call the API cross-origin ("" = none; the admin is same-origin)
	TrustProxy    bool   // behind a reverse proxy: take the client IP from X-Real-IP / X-Forwarded-For
	// TrustedProxies (TRUSTED_PROXIES) lists the peers (CIDRs / IPs) whose proxy
	// headers are believed when TrustProxy is on; empty = loopback + private ranges.
	TrustedProxies string
	// SiteURL (SITE_URL) is the public address, e.g. https://example.com
	// (e-mail links; HSTS is sent when it is https).
	SiteURL string
	// SetupToken (SETUP_TOKEN): when set, the first-run setup wizard requires it.
	SetupToken string

	// SiteCodeDisabled (SITE_CODE_DISABLED=true) stops every Site Code route and
	// event handler: the way back in when bad code locks the owner out.
	SiteCodeDisabled bool
	// CodeDSN (CODE_DB_DSN) is the Site Code database user on its own
	// database; empty = fallback mode (sc_ tables in the CMS database).
	CodeDSN string
	// SiteCodeSecretKey (SITE_CODE_SECRET_KEY) encrypts Site Code secrets;
	// empty = a key file site-code.key in PluginsDir.
	SiteCodeSecretKey string
	// PluginsDir (PLUGINS_DIR, default ./plugins-data) holds plugin packages
	// and site-code.key.
	PluginsDir string
	// SiteCodeMailSink=log (SITE_CODE_MAIL_SINK) writes mail.send messages to
	// the Site Code log instead of sending them (development).
	SiteCodeMailSink string

	// Demo mode (public playground): a shared visitor account and a periodic reset.
	DemoMode     bool
	DemoEmail    string
	DemoPassword string
	DemoReset    time.Duration // 0 = never reset automatically
}

func Load() Config {
	return Config{
		Addr:              env("ADDR", ":8090"),
		DSN:               env("DB_DSN", DefaultDSN),
		JWTSecret:         os.Getenv("JWT_SECRET"),
		UploadDir:         env("UPLOAD_DIR", "./uploads"),
		AdminDir:          env("ADMIN_DIR", "../frontend/dist/anasaki-studio/browser"),
		AdminEmail:        os.Getenv("ADMIN_EMAIL"),
		AdminPassword:     os.Getenv("ADMIN_PASSWORD"),
		CORSOrigin:        strings.TrimRight(strings.TrimSpace(os.Getenv("CORS_ORIGIN")), "/"), // off unless set: `ng serve` proxies /api (proxy.conf.json)
		TrustProxy:        isTrue(os.Getenv("TRUST_PROXY")),
		TrustedProxies:    strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")),
		SiteURL:           strings.TrimRight(strings.TrimSpace(os.Getenv("SITE_URL")), "/"),
		SetupToken:        strings.TrimSpace(os.Getenv("SETUP_TOKEN")),
		DemoMode:          isTrue(os.Getenv("DEMO_MODE")),
		SiteCodeDisabled:  isTrue(os.Getenv("SITE_CODE_DISABLED")),
		CodeDSN:           strings.TrimSpace(os.Getenv("CODE_DB_DSN")),
		SiteCodeSecretKey: os.Getenv("SITE_CODE_SECRET_KEY"),
		PluginsDir:        env("PLUGINS_DIR", "./plugins-data"),
		SiteCodeMailSink:  strings.ToLower(strings.TrimSpace(os.Getenv("SITE_CODE_MAIL_SINK"))),
		DemoEmail:         strings.ToLower(env("DEMO_EMAIL", "demo@example.com")),
		DemoPassword:      env("DEMO_PASSWORD", "demo1234"),
		DemoReset:         time.Duration(atoi(env("DEMO_RESET_MINUTES", "60"))) * time.Minute,
	}
}

// CheckJWTSecret refuses a missing or well-known JWT_SECRET (anyone could sign
// admin sessions) and warns about a short one.
func CheckJWTSecret(s string) (warning string, err error) {
	switch strings.TrimSpace(s) {
	case "", "dev-secret-change-me", "change-me", "changeme", "secret":
		return "", errors.New("JWT_SECRET is not set (or is a well-known placeholder): set it to a long random value, e.g. JWT_SECRET=$(openssl rand -hex 32)")
	}
	if len(s) < 32 {
		return "warning: JWT_SECRET is shorter than 32 characters — use a long random value (openssl rand -hex 32); changing it signs everyone out", nil
	}
	return "", nil
}

func isTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func atoi(v string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(v))
	if n < 0 {
		return 0
	}
	return n
}

// DefaultDSN is the development database (docker compose MySQL).
const DefaultDSN = "anasaki:anasaki@tcp(127.0.0.1:3306)/anasaki?parseTime=true&charset=utf8mb4&loc=UTC"

// Env returns the environment variable k, or def when it is empty.
func Env(k, def string) string { return env(k, def) }

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Package config membaca dan memvalidasi konfigurasi dari variabel lingkungan.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config adalah konfigurasi runtime aplikasi.
type Config struct {
	Env          string
	HTTPAddr     string
	DatabaseURL  string
	CookieSecure bool

	SessionIdleTTL     time.Duration
	SessionAbsoluteTTL time.Duration

	LoginMaxFailures   int
	LockoutBase        time.Duration
	LockoutMax         time.Duration
	LoginRatePerMinute int

	AdminAllowedCIDRs []netip.Prefix
	TrustedProxies    []netip.Prefix

	// WebDir adalah folder hasil build frontend (berisi index.html). Bila tidak
	// ada, server hanya menyajikan API.
	WebDir string
}

// IsProduction bernilai true bila APP_ENV=production.
func (c Config) IsProduction() bool { return c.Env == "production" }

// Load membaca konfigurasi lewat getenv (biasanya os.Getenv).
func Load(getenv func(string) string) (Config, error) {
	var errs []error
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}
	dur := func(key, def string) time.Duration {
		d, err := time.ParseDuration(get(key, def))
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("%s: durasi tidak valid", key))
		}
		return d
	}
	posInt := func(key, def string) int {
		n, err := strconv.Atoi(get(key, def))
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("%s: harus bilangan bulat positif", key))
		}
		return n
	}
	prefixes := func(key string) []netip.Prefix {
		var out []netip.Prefix
		for part := range strings.SplitSeq(get(key, ""), ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			p, err := netip.ParsePrefix(part)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: CIDR tidak valid %q", key, part))
				continue
			}
			out = append(out, p.Masked())
		}
		return out
	}

	c := Config{
		Env:         get("APP_ENV", "development"),
		HTTPAddr:    get("HTTP_ADDR", ":8080"),
		DatabaseURL: get("DATABASE_URL", ""),
		// ASSUMPTION: idle 8 jam dan mutlak 7 hari cocok untuk pola kerja JIEP.
		SessionIdleTTL:     dur("SESSION_IDLE_TTL", "8h"),
		SessionAbsoluteTTL: dur("SESSION_ABSOLUTE_TTL", "168h"),
		// ASSUMPTION: 5 kali gagal, kunci 1 menit berlipat ganda hingga 30 menit.
		LoginMaxFailures: posInt("LOGIN_MAX_FAILURES", "5"),
		LockoutBase:      dur("LOGIN_LOCKOUT_BASE", "1m"),
		LockoutMax:       dur("LOGIN_LOCKOUT_MAX", "30m"),
		// ASSUMPTION: 10 percobaan login per menit per IP.
		LoginRatePerMinute: posInt("LOGIN_RATE_PER_MINUTE", "10"),
		AdminAllowedCIDRs:  prefixes("ADMIN_ALLOWED_CIDRS"),
		TrustedProxies:     prefixes("TRUSTED_PROXIES"),
		WebDir:             get("WEB_DIR", filepath.Join("web", "dist")),
	}

	secure, err := strconv.ParseBool(get("SESSION_COOKIE_SECURE", "true"))
	if err != nil {
		errs = append(errs, errors.New("SESSION_COOKIE_SECURE: harus true atau false"))
	}
	c.CookieSecure = secure

	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL wajib diisi"))
	}
	if c.IsProduction() && !c.CookieSecure {
		errs = append(errs, errors.New("SESSION_COOKIE_SECURE tidak boleh false di production"))
	}
	if c.SessionIdleTTL > c.SessionAbsoluteTTL {
		errs = append(errs, errors.New("SESSION_IDLE_TTL tidak boleh melebihi SESSION_ABSOLUTE_TTL"))
	}
	if c.LockoutBase > c.LockoutMax {
		errs = append(errs, errors.New("LOGIN_LOCKOUT_BASE tidak boleh melebihi LOGIN_LOCKOUT_MAX"))
	}
	return c, errors.Join(errs...)
}

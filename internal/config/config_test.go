package config

import (
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(envOf(map[string]string{"DATABASE_URL": "postgres://x"}))
	require.NoError(t, err)
	assert.True(t, c.CookieSecure, "cookie Secure harus default true")
	assert.Equal(t, 8*time.Hour, c.SessionIdleTTL)
	assert.Equal(t, 5, c.LoginMaxFailures)
	assert.Empty(t, c.AdminAllowedCIDRs, "admin default tidak dibatasi")
	assert.Equal(t, []int{3, 50, 10, 25, 10},
		[]int{c.RiskDueDays, c.RiskDueProgress, c.HealthYellowPct, c.HealthRedPct, c.HealthBehindPoints})
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"tanpa DATABASE_URL", map[string]string{}},
		{"cookie tidak aman di production", map[string]string{
			"DATABASE_URL": "x", "APP_ENV": "production", "SESSION_COOKIE_SECURE": "false"}},
		{"CIDR rusak", map[string]string{"DATABASE_URL": "x", "ADMIN_ALLOWED_CIDRS": "10.0.0.0/33"}},
		{"durasi rusak", map[string]string{"DATABASE_URL": "x", "SESSION_IDLE_TTL": "abc"}},
		{"idle melebihi mutlak", map[string]string{
			"DATABASE_URL": "x", "SESSION_IDLE_TTL": "10h", "SESSION_ABSOLUTE_TTL": "1h"}},
		{"ambang nol", map[string]string{"DATABASE_URL": "x", "LOGIN_MAX_FAILURES": "0"}},
		{"kuning melebihi merah", map[string]string{"DATABASE_URL": "x", "HEALTH_YELLOW_PCT": "30", "HEALTH_RED_PCT": "20"}},
		{"persen di atas 100", map[string]string{"DATABASE_URL": "x", "RISK_DUE_PROGRESS": "150"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(envOf(tt.env))
			assert.Error(t, err)
		})
	}
}

func TestLoadCIDRs(t *testing.T) {
	c, err := Load(envOf(map[string]string{
		"DATABASE_URL":        "x",
		"ADMIN_ALLOWED_CIDRS": " 10.1.2.3/8 , 203.0.113.0/24",
	}))
	require.NoError(t, err)
	assert.Equal(t, []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("203.0.113.0/24"),
	}, c.AdminAllowedCIDRs)
}

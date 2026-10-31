package automation

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateWebhookURL(t *testing.T) {
	c := WebhookConfig{AllowedHosts: ParseAllowedHosts(" N8N.jiep.co.id , hooks.example.com:8443 ,")}
	assert.Equal(t, []string{"n8n.jiep.co.id", "hooks.example.com:8443"}, c.AllowedHosts)
	tests := []struct {
		url string
		ok  bool
	}{
		{"https://n8n.jiep.co.id/webhook/abc", true},
		{"https://N8N.JIEP.co.id:5678/webhook/abc?x=1", true},
		{"https://hooks.example.com:8443/h", true},
		{"https://hooks.example.com/h", false},   // port tidak sesuai entri
		{"http://n8n.jiep.co.id/webhook", false}, // bukan https
		{"https://evil.example/n8n.jiep.co.id", false},
		{"https://n8n.jiep.co.id.evil.example/", false},
		{"https://user:pass@n8n.jiep.co.id/", false},
		{"https://n8n.jiep.co.id/#frag", false},
		{"https://169.254.169.254/latest/meta-data", false},
		{"ftp://n8n.jiep.co.id/", false},
		{"bukan url", false},
		{"https://n8n.jiep.co.id/" + strings.Repeat("a", 2000), false},
	}
	for _, tt := range tests {
		t.Run(tt.url[:min(40, len(tt.url))], func(t *testing.T) {
			_, err := c.ValidateURL(tt.url)
			assert.Equal(t, tt.ok, err == nil, err)
		})
	}
	_, err := WebhookConfig{}.ValidateURL("https://n8n.jiep.co.id/")
	assert.ErrorIs(t, err, ErrWebhookDisabled)
}

func TestSignAndDeliver(t *testing.T) {
	secret, err := NewWebhookSecret()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(secret, "whsec_"))
	other, _ := NewWebhookSecret()
	assert.NotEqual(t, secret, other)

	now := time.Unix(1_793_000_000, 0)
	body := []byte(`{"a":1}`)
	sig := Sign(secret, now.Unix(), body)
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte("1793000000." + string(body)))
	assert.Equal(t, "sha256="+hex.EncodeToString(m.Sum(nil)), sig)

	var got http.Header
	var gotBody []byte
	status := http.StatusNoContent
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		gotBody, _ = io.ReadAll(r.Body)
		if status == http.StatusFound {
			http.Redirect(w, r, "https://169.254.169.254/", http.StatusFound)
			return
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	client := NewWebhookClient()
	client.Transport = srv.Client().Transport

	code, err := deliver(context.Background(), client, srv.URL+"/hook", secret, 42, body, now)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, code)
	assert.Equal(t, body, gotBody)
	assert.Equal(t, "42", got.Get("X-JWM-Delivery"))
	assert.Equal(t, strconv.FormatInt(now.Unix(), 10), got.Get("X-JWM-Timestamp"))
	assert.Equal(t, sig, got.Get("X-JWM-Signature"))
	assert.Equal(t, "application/json", got.Get("Content-Type"))

	status = http.StatusInternalServerError
	code, err = deliver(context.Background(), client, srv.URL, secret, 1, body, now)
	assert.Equal(t, http.StatusInternalServerError, code)
	assert.ErrorContains(t, err, "HTTP 500")

	// Redirect tidak diikuti: dianggap gagal.
	status = http.StatusFound
	code, err = deliver(context.Background(), client, srv.URL, secret, 1, body, now)
	assert.Equal(t, http.StatusFound, code)
	assert.Error(t, err)
}

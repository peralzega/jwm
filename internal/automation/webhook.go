package automation

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// WebhookConfig mengatur aksi "panggil webhook".
type WebhookConfig struct {
	// AllowedHosts: host (opsional dengan port) yang boleh dituju, huruf kecil.
	// Kosong berarti aksi webhook dinonaktifkan (cegah SSRF ke jaringan internal).
	AllowedHosts []string
	// BaseURL aplikasi untuk tautan item di payload, mis. https://jwm.jiep.co.id.
	BaseURL string
}

// Galat webhook.
var (
	ErrWebhookDisabled = errors.New("aksi webhook dinonaktifkan: WEBHOOK_ALLOWED_HOSTS belum diisi")
	ErrWebhookURL      = errors.New("URL webhook harus https:// ke host yang ada di daftar izin")
)

// Batas kiriman webhook (keputusan M7).
const (
	WebhookTimeout = 10 * time.Second
	// WebhookAttempts: 1 kiriman + 3 percobaan ulang.
	WebhookAttempts = 4
	maxURLLen       = 2000
)

// webhookBackoff: jeda sebelum percobaan ulang ke-1, ke-2, dan ke-3.
var webhookBackoff = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}

// ParseAllowedHosts membaca daftar host dipisah koma, mis. "n8n.jiep.co.id,hooks.example.com:8443".
func ParseAllowedHosts(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if h := strings.ToLower(strings.TrimSpace(part)); h != "" {
			out = append(out, h)
		}
	}
	return out
}

// ValidateURL memastikan URL webhook https, tanpa kredensial, dan host-nya
// diizinkan. Entri daftar tanpa port mengizinkan host itu di port mana pun.
func (c WebhookConfig) ValidateURL(raw string) (string, error) {
	if len(c.AllowedHosts) == 0 {
		return "", ErrWebhookDisabled
	}
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || len(raw) > maxURLLen || u.Scheme != "https" || u.User != nil || u.Host == "" || u.Fragment != "" {
		return "", ErrWebhookURL
	}
	host := strings.ToLower(u.Host)
	if !slices.Contains(c.AllowedHosts, host) && !slices.Contains(c.AllowedHosts, strings.ToLower(u.Hostname())) {
		return "", fmt.Errorf("%w (host %q tidak diizinkan)", ErrWebhookURL, u.Hostname())
	}
	return u.String(), nil
}

// NewWebhookSecret membuat rahasia HMAC acak 256-bit.
func NewWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("buat rahasia: %w", err)
	}
	return "whsec_" + base64.RawURLEncoding.EncodeToString(b), nil
}

// Sign menghitung tanda tangan HMAC-SHA256 atas "<timestamp>.<body>".
// Penerima (mis. n8n) memverifikasi dengan rahasia yang sama dan menolak
// timestamp yang terlalu lama untuk mencegah pengulangan.
func Sign(secret string, timestamp int64, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(m, "%d.", timestamp)
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// NewWebhookClient membuat klien HTTP yang tidak mengikuti redirect (redirect
// bisa mengarah ke host di luar daftar izin) dengan batas waktu 10 detik.
func NewWebhookClient() *http.Client {
	return &http.Client{
		Timeout:       WebhookTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// deliver mengirim satu payload. Berhasil hanya untuk status 2xx.
func deliver(ctx context.Context, client *http.Client, target, secret string, deliveryID int64, body []byte, now time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, WebhookTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("buat permintaan: %w", err)
	}
	ts := now.Unix()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "JWM-Webhook/1")
	req.Header.Set("X-JWM-Delivery", strconv.FormatInt(deliveryID, 10))
	req.Header.Set("X-JWM-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-JWM-Signature", Sign(secret, ts, body))
	res, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("kirim: %w", err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return res.StatusCode, fmt.Errorf("penerima menjawab HTTP %d", res.StatusCode)
	}
	return res.StatusCode, nil
}

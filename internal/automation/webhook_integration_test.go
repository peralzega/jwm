package automation_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jwm/internal/auth"
	"jwm/internal/automation"
)

// receiver adalah server HTTPS uji yang mencatat kiriman dan menjawab status tertentu.
type receiver struct {
	srv    *httptest.Server
	mu     sync.Mutex
	status int
	got    []delivery
}

type delivery struct {
	header http.Header
	body   []byte
}

func newReceiver(t *testing.T) *receiver {
	rc := &receiver{status: http.StatusOK}
	rc.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rc.mu.Lock()
		rc.got = append(rc.got, delivery{r.Header.Clone(), b})
		status := rc.status
		rc.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(rc.srv.Close)
	return rc
}

func (rc *receiver) host() string {
	u, _ := url.Parse(rc.srv.URL)
	return u.Host
}

func (rc *receiver) deliveries() []delivery {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return append([]delivery{}, rc.got...)
}

func webhookFixture(t *testing.T) (*fixture, *receiver) {
	rc := newReceiver(t)
	fx := newFixtureWith(t, automation.WebhookConfig{
		AllowedHosts: automation.ParseAllowedHosts(rc.host()), BaseURL: "https://jwm.jiep.test/",
	})
	fx.worker.Client.Transport = rc.srv.Client().Transport
	return fx, rc
}

func (fx *fixture) deliveriesOf(rule string) []map[string]any {
	fx.t.Helper()
	r := fx.must(fx.pm, http.MethodGet, "/api/v1/automations/"+rule+"/webhooks", nil, http.StatusOK)
	var out []map[string]any
	for _, x := range r.Body["items"].([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}

func TestWebhookRuleValidationAndSecret(t *testing.T) {
	fx, rc := webhookFixture(t)
	base := "/api/v1/boards/" + fx.board + "/automations"
	hook := func(u string) map[string]any {
		return map[string]any{"nama": "Kabari n8n", "pemicu": statusTrigger("done"), "aktif": true,
			"aksi": []any{map[string]any{"jenis": "panggil_webhook", "url": u}}}
	}
	for _, bad := range []string{
		"http://" + rc.host() + "/hook",
		"https://lain.example/hook",
		"https://169.254.169.254/latest/meta-data",
		"https://user:pw@" + rc.host() + "/hook",
	} {
		r := fx.pm.Do(http.MethodPost, base, hook(bad))
		assert.Equal(t, http.StatusUnprocessableEntity, r.Status, bad)
	}

	r := fx.must(fx.pm, http.MethodPost, base, hook(rc.srv.URL+"/hook"), http.StatusCreated)
	id := r.Body["id"].(string)
	secret, ok := r.Body["webhook_rahasia"].(string)
	require.True(t, ok, "rahasia dikembalikan sekali saat dibuat")
	assert.Regexp(t, `^whsec_[A-Za-z0-9_-]{43}$`, secret)

	// Tidak pernah tampil lagi: daftar, ubah tanpa ganti, dan audit.
	list := fx.must(fx.pm, http.MethodGet, base, nil, http.StatusOK)
	raw, _ := json.Marshal(list.Body)
	assert.NotContains(t, string(raw), secret)
	r = fx.must(fx.pm, http.MethodPatch, "/api/v1/automations/"+id, hook(rc.srv.URL+"/hook2"), http.StatusOK)
	assert.Nil(t, r.Body["webhook_rahasia"])
	var audit string
	require.NoError(t, fx.pool.QueryRow(context.Background(),
		"SELECT string_agg(coalesce(sesudah::text, ''), ' ') FROM audit_log").Scan(&audit))
	assert.NotContains(t, audit, secret)

	// Ganti rahasia: hanya manager; rahasia baru berbeda.
	_, member := fx.member(auth.RoleMember, "member")
	assert.Equal(t, http.StatusForbidden, member.Do(http.MethodPost, "/api/v1/automations/"+id+"/webhook-secret", nil).Status)
	r = fx.must(fx.pm, http.MethodPost, "/api/v1/automations/"+id+"/webhook-secret", nil, http.StatusOK)
	assert.NotEqual(t, secret, r.Body["webhook_rahasia"])

	// Aksi webhook dihapus dari aturan: rahasia ikut dihapus.
	noHook := map[string]any{"nama": "Tanpa webhook", "pemicu": statusTrigger("done"), "aktif": true,
		"aksi": []any{map[string]any{"jenis": "ubah_kolom", "column_id": fx.prio, "nilai": "rendah"}}}
	fx.must(fx.pm, http.MethodPatch, "/api/v1/automations/"+id, noHook, http.StatusOK)
	var n int
	require.NoError(t, fx.pool.QueryRow(context.Background(), "SELECT count(*) FROM automation_secrets").Scan(&n))
	assert.Zero(t, n)
	assert.Equal(t, http.StatusUnprocessableEntity, fx.pm.Do(http.MethodPost, "/api/v1/automations/"+id+"/webhook-secret", nil).Status)

	// Tanpa daftar izin, aksi webhook ditolak.
	plain := newFixture(t)
	r = plain.pm.Do(http.MethodPost, "/api/v1/boards/"+plain.board+"/automations", hook("https://n8n.jiep.co.id/hook"))
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status)
	assert.Contains(t, r.Body["message"], "WEBHOOK_ALLOWED_HOSTS")
}

func TestWebhookDeliverySignedWithRetry(t *testing.T) {
	fx, rc := webhookFixture(t)
	r := fx.must(fx.pm, http.MethodPost, "/api/v1/boards/"+fx.board+"/automations", map[string]any{
		"nama": "Kabari n8n", "pemicu": statusTrigger("done"), "aktif": true,
		"aksi": []any{map[string]any{"jenis": "panggil_webhook", "url": rc.srv.URL + "/hook"}},
	}, http.StatusCreated)
	rule := r.Body["id"].(string)
	secret := r.Body["webhook_rahasia"].(string)

	a := fx.item("Integrasi")
	fx.set(a, fx.status, "done")
	fx.drain()
	assert.Equal(t, "webhook diantrekan", fx.runs(rule)[0]["pesan"])

	// Penerima gagal: dicoba ulang dengan jeda, lalu berhasil.
	rc.status = http.StatusBadGateway
	n, err := fx.worker.DispatchWebhooks(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	d := fx.deliveriesOf(rule)[0]
	assert.Equal(t, "menunggu", d["status"])
	assert.Equal(t, float64(502), d["kode_http"])
	assert.Equal(t, float64(1), d["percobaan"])
	// Belum jatuh tempo (jeda 30 detik): tidak dikirim lagi.
	n, _ = fx.worker.DispatchWebhooks(context.Background())
	assert.Zero(t, n)
	_, err = fx.pool.Exec(context.Background(), "UPDATE webhook_outbox SET berikutnya = now()")
	require.NoError(t, err)
	rc.status = http.StatusOK
	n, _ = fx.worker.DispatchWebhooks(context.Background())
	assert.Equal(t, 1, n)
	d = fx.deliveriesOf(rule)[0]
	assert.Equal(t, "terkirim", d["status"])
	assert.Equal(t, float64(2), d["percobaan"])

	// Tanda tangan dapat diverifikasi penerima; payload berisi data minimal.
	got := rc.deliveries()
	require.Len(t, got, 2)
	last := got[1]
	ts, err := strconv.ParseInt(last.header.Get("X-JWM-Timestamp"), 10, 64)
	require.NoError(t, err)
	assert.Equal(t, automation.Sign(secret, ts, last.body), last.header.Get("X-JWM-Signature"))
	var p map[string]any
	require.NoError(t, json.Unmarshal(last.body, &p))
	assert.Equal(t, "jwm.otomasi", p["jenis"])
	assert.Equal(t, map[string]any{"id": a, "judul": "Integrasi", "url": "https://jwm.jiep.test/board/" + fx.board}, p["item"])
	assert.Equal(t, "status_berubah", p["pemicu"].(map[string]any)["jenis"])
	assert.Equal(t, "done", p["pemicu"].(map[string]any)["sesudah"])
	assert.Equal(t, "Otomasi", p["proyek"].(map[string]any)["nama"])
	assert.NotContains(t, string(last.body), "nilai")
}

func TestWebhookGivesUpAfterRetriesAndRevokedHost(t *testing.T) {
	fx, rc := webhookFixture(t)
	r := fx.must(fx.pm, http.MethodPost, "/api/v1/boards/"+fx.board+"/automations", map[string]any{
		"nama": "Kabari n8n", "pemicu": map[string]any{"jenis": "item_dibuat"}, "aktif": true,
		"aksi": []any{map[string]any{"jenis": "panggil_webhook", "url": rc.srv.URL + "/hook"}},
	}, http.StatusCreated)
	rule := r.Body["id"].(string)
	rc.status = http.StatusInternalServerError
	fx.item("Satu")
	fx.drain()
	for range automation.WebhookAttempts {
		_, err := fx.pool.Exec(context.Background(), "UPDATE webhook_outbox SET berikutnya = now()")
		require.NoError(t, err)
		n, err := fx.worker.DispatchWebhooks(context.Background())
		require.NoError(t, err)
		require.Equal(t, 1, n)
	}
	d := fx.deliveriesOf(rule)[0]
	assert.Equal(t, "gagal", d["status"])
	assert.Equal(t, float64(automation.WebhookAttempts), d["percobaan"])
	assert.Contains(t, d["galat"], "HTTP 500")
	assert.Len(t, rc.deliveries(), automation.WebhookAttempts)

	// Host dicabut dari daftar izin: kiriman yang mengantre gagal tanpa dikirim,
	// dan aturan berikutnya gagal saat dijalankan.
	rc.status = http.StatusOK
	fx.item("Dua")
	fx.drain()
	fx.worker.Webhook.AllowedHosts = []string{"n8n.jiep.co.id"}
	n, err := fx.worker.DispatchWebhooks(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	d = fx.deliveriesOf(rule)[0]
	assert.Equal(t, "gagal", d["status"])
	assert.Contains(t, d["galat"], "tidak diizinkan")
	assert.Len(t, rc.deliveries(), automation.WebhookAttempts, "tidak ada kiriman ke host yang dicabut")
	fx.item("Tiga")
	fx.drain()
	assert.Equal(t, "gagal", fx.runs(rule)[0]["hasil"])
}

package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

// Client adalah klien HTTP tes dengan cookie jar dan token CSRF otomatis.
type Client struct {
	t    *testing.T
	base string
	http *http.Client
	CSRF string
}

// NewClient membuat klien untuk server di base (mis. httptest.Server.URL).
func NewClient(t *testing.T, base string) *Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &Client{t: t, base: base, http: &http.Client{Jar: jar}}
}

// Resp adalah respons yang sudah dibaca.
type Resp struct {
	Status int
	Body   map[string]any
	Raw    string
	Header http.Header
}

// Do mengirim permintaan JSON. Token CSRF dari respons disimpan untuk permintaan berikutnya.
func (c *Client) Do(method, path string, body any) Resp {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		c.t.Fatalf("buat permintaan: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.CSRF != "" {
		req.Header.Set("X-CSRF-Token", c.CSRF)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("kirim permintaan: %v", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		c.t.Fatalf("baca respons: %v", err)
	}
	out := Resp{Status: res.StatusCode, Raw: string(raw), Header: res.Header}
	_ = json.Unmarshal(raw, &out.Body)
	if tok, ok := out.Body["csrf_token"].(string); ok {
		c.CSRF = tok
	}
	return out
}

// Login masuk lewat /api/v1/auth/login.
func (c *Client) Login(email, password string) Resp {
	c.t.Helper()
	return c.Do(http.MethodPost, "/api/v1/auth/login", map[string]string{"email": email, "password": password})
}

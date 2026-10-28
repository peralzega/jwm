package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
)

const tokenBytes = 32 // 256 bit

// newToken membuat token sesi acak 256-bit dalam base64url.
func newToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("buat token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashToken menghasilkan SHA-256 token. Hanya hash yang disimpan di database.
func hashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

// CSRFToken diturunkan dari token sesi dengan pemisah domain. Penyerang tidak
// dapat menghitungnya tanpa cookie sesi (HttpOnly), jadi tidak perlu disimpan.
func CSRFToken(sessionToken string) string {
	sum := sha256.Sum256([]byte("jwm-csrf-v1:" + sessionToken))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func validCSRF(sessionToken, got string) bool {
	want := CSRFToken(sessionToken)
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

package auth

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Argon2Params adalah parameter argon2id.
type Argon2Params struct {
	Memory  uint32 // KiB
	Time    uint32
	Threads uint8
	SaltLen uint32
	KeyLen  uint32
}

// ASSUMPTION: 64 MiB, t=3, p=2 (di atas rekomendasi minimum OWASP). Setel ulang sesuai server.
var DefaultArgon2Params = Argon2Params{Memory: 64 * 1024, Time: 3, Threads: 2, SaltLen: 16, KeyLen: 32}

var errMalformedHash = errors.New("format hash argon2id tidak valid")

// Hasher membuat dan memverifikasi hash kata sandi argon2id dalam format PHC.
type Hasher struct {
	p     Argon2Params
	dummy string // hash palsu untuk menyamakan waktu respons saat akun tidak ada
}

// NewHasher membuat Hasher dengan parameter p.
func NewHasher(p Argon2Params) (*Hasher, error) {
	h := &Hasher{p: p}
	d, err := h.Hash(rand.Text())
	if err != nil {
		return nil, err
	}
	h.dummy = d
	return h, nil
}

// Hash menghasilkan string $argon2id$v=19$m=..,t=..,p=..$salt$hash.
func (h *Hasher) Hash(password string) (string, error) {
	salt := make([]byte, h.p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("buat salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, h.p.Time, h.p.Memory, h.p.Threads, h.p.KeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, h.p.Memory, h.p.Time, h.p.Threads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// Verify membandingkan password dengan hash terenkode secara waktu-konstan.
func (h *Hasher) Verify(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errMalformedHash
	}
	var p Argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil {
		return false, errMalformedHash
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false, errMalformedHash
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false, errMalformedHash
	}
	got := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// VerifyDummy menjalankan verifikasi terhadap hash palsu agar waktu respons
// untuk akun yang tidak ada setara dengan akun yang ada.
func (h *Hasher) VerifyDummy(password string) {
	_, _ = h.Verify(password, h.dummy)
}

// Batas panjang kata sandi (dalam karakter).
const (
	MinPasswordLen = 12
	MaxPasswordLen = 128
)

// Galat kebijakan kata sandi.
var (
	ErrPasswordTooShort = errors.New("kata sandi minimal 12 karakter")
	ErrPasswordTooLong  = errors.New("kata sandi maksimal 128 karakter")
	ErrPasswordCommon   = errors.New("kata sandi terlalu umum atau mudah ditebak")
)

// Daftar sandi umum (>= 12 karakter, huruf kecil) dari SecLists, lisensi MIT.
// Sumber: Passwords/Common-Credentials/100k-most-used-passwords-NCSC.txt dan
// xato-net-10-million-passwords-100000.txt, https://github.com/danielmiessler/SecLists
//
//go:embed common_passwords.txt
var commonPasswordsRaw []byte

var commonPasswords = func() map[string]struct{} {
	m := make(map[string]struct{})
	s := bufio.NewScanner(bytes.NewReader(commonPasswordsRaw))
	for s.Scan() {
		if line := strings.TrimSpace(s.Text()); line != "" {
			m[line] = struct{}{}
		}
	}
	return m
}()

// ValidatePassword memeriksa kebijakan sandi: panjang, daftar sandi umum,
// dan kemiripan dengan email.
func ValidatePassword(password, email string) error {
	n := utf8.RuneCountInString(password)
	switch {
	case n < MinPasswordLen:
		return ErrPasswordTooShort
	case n > MaxPasswordLen:
		return ErrPasswordTooLong
	}
	lower := strings.ToLower(password)
	if _, ok := commonPasswords[lower]; ok {
		return ErrPasswordCommon
	}
	if local, _, _ := strings.Cut(strings.ToLower(email), "@"); len(local) >= 4 && strings.Contains(lower, local) {
		return ErrPasswordCommon
	}
	// ASSUMPTION: satu karakter berulang atau nama aplikasi bukan sandi yang layak.
	if strings.Count(lower, string([]rune(lower)[0])) == n {
		return ErrPasswordCommon
	}
	for _, w := range []string{"jiep", "jwm"} {
		if rest, ok := strings.CutPrefix(lower, w); ok && strings.Trim(rest, "0123456789!@#$%^&*._-") == "" {
			return ErrPasswordCommon
		}
	}
	return nil
}

package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testParams murah agar tes cepat; produksi memakai DefaultArgon2Params.
var testParams = Argon2Params{Memory: 1024, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}

func TestHashIsArgon2idNotPlaintext(t *testing.T) {
	h, err := NewHasher(testParams)
	require.NoError(t, err)
	const pw = "kuda-lari-di-sawah-42"

	a, err := h.Hash(pw)
	require.NoError(t, err)
	b, err := h.Hash(pw)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(a, "$argon2id$v=19$m=1024,t=1,p=1$"), a)
	assert.NotContains(t, a, pw)
	assert.NotEqual(t, a, b, "salt harus acak")

	ok, err := h.Verify(pw, a)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = h.Verify(pw+"x", a)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestVerifyUsesParamsFromHash(t *testing.T) {
	strong, err := NewHasher(Argon2Params{Memory: 2048, Time: 2, Threads: 1, SaltLen: 16, KeyLen: 32})
	require.NoError(t, err)
	weak, err := NewHasher(testParams)
	require.NoError(t, err)
	enc, err := strong.Hash("sandi-panjang-sekali")
	require.NoError(t, err)
	ok, err := weak.Verify("sandi-panjang-sekali", enc)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestVerifyMalformed(t *testing.T) {
	h, err := NewHasher(testParams)
	require.NoError(t, err)
	for _, enc := range []string{
		"",
		"plaintext",
		"$argon2i$v=19$m=1024,t=1,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=18$m=1024,t=1,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=x,t=1,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=1024,t=1,p=1$!!!$aGFzaA",
		"$argon2id$v=19$m=1024,t=1,p=1$c2FsdA$",
	} {
		_, err := h.Verify("apa saja", enc)
		assert.ErrorIs(t, err, errMalformedHash, enc)
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name  string
		pw    string
		email string
		want  error
	}{
		{"valid", "kuda-lari-di-sawah", "budi@jiep.co.id", nil},
		{"valid unicode 12 karakter", "ééééééééééé1", "budi@jiep.co.id", nil},
		{"terlalu pendek", "pendek123", "budi@jiep.co.id", ErrPasswordTooShort},
		{"11 karakter", "abcdefghijk", "budi@jiep.co.id", ErrPasswordTooShort},
		{"terlalu panjang", strings.Repeat("ab", 65), "budi@jiep.co.id", ErrPasswordTooLong},
		{"daftar umum", "password1234", "budi@jiep.co.id", ErrPasswordCommon},
		{"daftar umum huruf besar", "PASSWORD1234", "budi@jiep.co.id", ErrPasswordCommon},
		{"memuat bagian lokal email", "xxbudisantoso99", "budisantoso@jiep.co.id", ErrPasswordCommon},
		{"satu karakter berulang", "aaaaaaaaaaaa", "budi@jiep.co.id", ErrPasswordCommon},
		{"nama aplikasi", "jiep12345678", "budi@jiep.co.id", ErrPasswordCommon},
		{"mengandung kata lain tidak ditolak", "pipe-dan-kabel", "budi@jiep.co.id", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.pw, tt.email)
			if tt.want == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestCommonPasswordListLoaded(t *testing.T) {
	assert.Greater(t, len(commonPasswords), 1000)
}

func TestTokens(t *testing.T) {
	a, err := newToken()
	require.NoError(t, err)
	b, err := newToken()
	require.NoError(t, err)
	assert.Len(t, a, 43, "32 byte base64url tanpa padding")
	assert.NotEqual(t, a, b)
	assert.Len(t, hashToken(a), 32)
	assert.NotEqual(t, CSRFToken(a), CSRFToken(b))
	assert.True(t, validCSRF(a, CSRFToken(a)))
	assert.False(t, validCSRF(a, CSRFToken(b)))
	assert.False(t, validCSRF(a, ""))
}

func TestLockoutDuration(t *testing.T) {
	s := Settings{MaxFailures: 5, LockoutBase: time.Minute, LockoutMax: 30 * time.Minute}
	tests := []struct {
		failures int
		want     time.Duration
	}{
		{0, 0},
		{4, 0},
		{5, time.Minute},
		{9, time.Minute},
		{10, 2 * time.Minute},
		{15, 4 * time.Minute},
		{25, 16 * time.Minute},
		{30, 30 * time.Minute},
		{500, 30 * time.Minute},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, LockoutDuration(tt.failures, s), "gagal=%d", tt.failures)
	}
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"Budi@JIEP.co.id ", "budi@jiep.co.id", false},
		{"bukan-email", "", true},
		{"Budi <budi@jiep.co.id>", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		got, err := ValidateEmail(tt.in)
		assert.Equal(t, tt.wantErr, err != nil, tt.in)
		assert.Equal(t, tt.want, got)
	}
}

func TestRole(t *testing.T) {
	assert.True(t, RoleViewer.Valid())
	assert.False(t, Role("superuser").Valid())
	assert.True(t, RoleAdmin.IsAdmin())
	assert.False(t, RolePMO.IsAdmin())
}

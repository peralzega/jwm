package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunRefusals(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		args    []string
		wantErr string
	}{
		{"production ditolak", map[string]string{"APP_ENV": "production", "DATABASE_URL": "postgres://x"}, []string{"-email", "a@jiep.test"}, "production"},
		{"production huruf besar ditolak", map[string]string{"APP_ENV": "PRODUCTION", "DATABASE_URL": "postgres://x"}, []string{"-email", "a@jiep.test"}, "production"},
		{"tanpa DATABASE_URL", map[string]string{"APP_ENV": "development", "DATABASE_URL": ""}, []string{"-email", "a@jiep.test"}, "wajib diisi"},
		{"tanpa email", map[string]string{"APP_ENV": "development", "DATABASE_URL": "postgres://x", "BOOTSTRAP_ADMIN_EMAIL": ""}, nil, "wajib diisi"},
		{"flag tak dikenal", map[string]string{"APP_ENV": "development"}, []string{"-hapus-semua"}, "flag provided but not defined"},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			err := run(context.Background(), log, tt.args)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

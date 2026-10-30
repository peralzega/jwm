package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDensityValid(t *testing.T) {
	tests := []struct {
		in   Density
		want bool
	}{
		{DensityCompact, true},
		{DensityNormal, true},
		{"", false},
		{"Compact", false},
		{"lebar", false},
	}
	for _, tt := range tests {
		t.Run(string(tt.in), func(t *testing.T) {
			assert.Equal(t, tt.want, tt.in.Valid())
		})
	}
}

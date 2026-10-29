package board

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeConfig(t *testing.T) {
	tests := []struct {
		name    string
		tipe    ColumnType
		in      string
		want    string // kosong = tidak diperiksa
		wantErr error
	}{
		{"status default", TypeStatus, ``, "", nil},
		{"status kustom", TypeStatus, `{"labels":[{"id":"a","judul":"A","warna":"biru"}]}`,
			`{"labels":[{"id":"a","judul":"A","warna":"biru"}]}`, nil},
		{"status tanpa label", TypeStatus, `{"labels":[]}`, "", ErrInvalidConfig},
		{"status id ganda", TypeStatus, `{"labels":[{"id":"a","judul":"A","warna":"biru"},{"id":"a","judul":"B","warna":"biru"}]}`, "", ErrInvalidConfig},
		{"status id huruf besar", TypeStatus, `{"labels":[{"id":"A","judul":"A","warna":"biru"}]}`, "", ErrInvalidConfig},
		{"status warna asing", TypeStatus, `{"labels":[{"id":"a","judul":"A","warna":"#ff0000"}]}`, "", ErrInvalidConfig},
		{"status field asing", TypeStatus, `{"labels":[{"id":"a","judul":"A","warna":"biru"}],"x":1}`, "", ErrInvalidConfig},
		{"number satuan", TypeNumber, `{"satuan":"jam"}`, `{"satuan":"jam"}`, nil},
		{"number kosong", TypeNumber, ``, `{}`, nil},
		{"text tanpa konfigurasi", TypeText, `{}`, `{}`, nil},
		{"text dengan konfigurasi", TypeText, `{"a":1}`, "", ErrInvalidConfig},
		{"tipe asing", ColumnType("formula"), ``, "", ErrInvalidColumnType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeConfig(tt.tipe, json.RawMessage(tt.in))
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.want != "" {
				assert.JSONEq(t, tt.want, string(got))
			}
		})
	}

	got, err := NormalizeConfig(TypeStatus, nil)
	require.NoError(t, err)
	ids, err := statusLabelIDs(got)
	require.NoError(t, err)
	assert.Equal(t, []string{"ready_to_start", "in_progress", "waiting", "done"}, ids)
}

func TestNormalizeValue(t *testing.T) {
	member := uuid.MustParse("01900000-0000-7000-8000-000000000001")
	outsider := uuid.MustParse("01900000-0000-7000-8000-000000000002")
	statusCfg, _ := json.Marshal(DefaultStatusConfig())
	vc := ValueContext{Config: statusCfg, IsMember: func(id uuid.UUID) bool { return id == member }}

	tests := []struct {
		name    string
		tipe    ColumnType
		in      string
		want    string
		wantErr error
	}{
		{"text", TypeText, `"halo"`, `"halo"`, nil},
		{"text bukan string", TypeText, `12`, "", ErrInvalidValue},
		{"text terlalu panjang", TypeText, `"` + strings.Repeat("a", 2001) + `"`, "", ErrInvalidValue},
		{"status valid", TypeStatus, `"done"`, `"done"`, nil},
		{"status asing", TypeStatus, `"selesai"`, "", ErrInvalidValue},
		{"prioritas", TypePriority, `"kritis"`, `"kritis"`, nil},
		{"prioritas asing", TypePriority, `"urgent"`, "", ErrInvalidValue},
		{"person anggota, duplikat dibuang", TypePerson, `["` + member.String() + `","` + member.String() + `"]`, `["` + member.String() + `"]`, nil},
		{"person kosong", TypePerson, `[]`, `[]`, nil},
		{"person bukan anggota", TypePerson, `["` + outsider.String() + `"]`, "", ErrInvalidValue},
		{"person bukan UUID", TypePerson, `["budi"]`, "", ErrInvalidValue},
		{"tanggal", TypeDate, `"2026-02-28"`, `"2026-02-28"`, nil},
		{"tanggal mustahil", TypeDate, `"2026-02-30"`, "", ErrInvalidValue},
		{"timeline", TypeTimeline, `{"mulai":"2026-01-01","selesai":"2026-01-31"}`, `{"mulai":"2026-01-01","selesai":"2026-01-31"}`, nil},
		{"timeline sehari", TypeTimeline, `{"mulai":"2026-01-01","selesai":"2026-01-01"}`, `{"mulai":"2026-01-01","selesai":"2026-01-01"}`, nil},
		{"timeline terbalik", TypeTimeline, `{"mulai":"2026-02-01","selesai":"2026-01-31"}`, "", ErrInvalidValue},
		{"timeline field asing", TypeTimeline, `{"mulai":"2026-01-01","selesai":"2026-01-31","x":1}`, "", ErrInvalidValue},
		{"angka", TypeNumber, `12.5`, `12.5`, nil},
		{"angka string", TypeNumber, `"12"`, "", ErrInvalidValue},
		{"angka raksasa", TypeNumber, `1e300`, "", ErrInvalidValue},
		{"progres", TypeProgress, `75`, `75`, nil},
		{"progres 0 dan 100 sah", TypeProgress, `100`, `100`, nil},
		{"progres pecahan", TypeProgress, `50.5`, "", ErrInvalidValue},
		{"progres di atas 100", TypeProgress, `101`, "", ErrInvalidValue},
		{"progres negatif", TypeProgress, `-1`, "", ErrInvalidValue},
		{"dependency hanya-baca", TypeDependency, `[]`, "", ErrReadOnlyColumn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeValue(tt.tipe, json.RawMessage(tt.in), vc)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(got))
		})
	}
}

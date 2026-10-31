// Package board menangani board, kolom bertipe, grup, dan item.
package board

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ColumnType adalah tipe kolom v1 (SPEC bagian 3).
type ColumnType string

// Tipe kolom.
const (
	TypeText       ColumnType = "text"
	TypeStatus     ColumnType = "status"
	TypePerson     ColumnType = "person"
	TypeDate       ColumnType = "date"
	TypeTimeline   ColumnType = "timeline"
	TypeNumber     ColumnType = "number"
	TypeProgress   ColumnType = "progress"
	TypePriority   ColumnType = "priority"
	TypeDependency ColumnType = "dependency"
)

// Valid bernilai true untuk tipe kolom yang dikenal.
func (t ColumnType) Valid() bool {
	switch t {
	case TypeText, TypeStatus, TypePerson, TypeDate, TypeTimeline,
		TypeNumber, TypeProgress, TypePriority, TypeDependency:
		return true
	}
	return false
}

// Galat validasi kolom dan nilai.
var (
	ErrInvalidColumnType = errors.New("tipe kolom tidak dikenal")
	ErrInvalidConfig     = errors.New("konfigurasi kolom tidak valid")
	ErrInvalidValue      = errors.New("nilai tidak sesuai tipe kolom")
	ErrReadOnlyColumn    = errors.New("kolom dependency dikelola lewat dependensi item, bukan nilai sel")
	ErrInvalidColor      = errors.New("warna tidak dikenal")
)

// ASSUMPTION: palet warna berupa nama token; frontend memetakannya ke warna tema.
var colors = []string{"abu", "biru", "hijau", "kuning", "oranye", "merah", "ungu", "tosca", "merah_muda"}

// ValidColor bernilai true untuk nama token warna yang dikenal.
func ValidColor(c string) bool { return slices.Contains(colors, c) }

var labelIDPattern = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)

// StatusLabel adalah satu pilihan pada kolom status.
type StatusLabel struct {
	ID    string `json:"id"`
	Judul string `json:"judul"`
	Warna string `json:"warna"`
}

// StatusConfig adalah konfigurasi kolom status.
type StatusConfig struct {
	Labels []StatusLabel `json:"labels"`
}

// NumberConfig adalah konfigurasi kolom angka.
type NumberConfig struct {
	Satuan string `json:"satuan,omitempty"`
}

// DefaultStatusConfig adalah label status bawaan (SPEC M2).
func DefaultStatusConfig() StatusConfig {
	return StatusConfig{Labels: []StatusLabel{
		{ID: "ready_to_start", Judul: "Ready to start", Warna: "abu"},
		{ID: "in_progress", Judul: "In progress", Warna: "oranye"},
		{ID: "waiting", Judul: "Waiting", Warna: "merah"},
		{ID: "done", Judul: "Done", Warna: "hijau"},
	}}
}

func decodeStrict(raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func isEmptyJSON(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s == "" || s == "null"
}

// NormalizeConfig memvalidasi konfigurasi kolom dan mengembalikan bentuk kanonisnya.
// Konfigurasi kosong untuk kolom status diisi label bawaan.
func NormalizeConfig(t ColumnType, raw json.RawMessage) (json.RawMessage, error) {
	if !t.Valid() {
		return nil, ErrInvalidColumnType
	}
	switch t {
	case TypeStatus:
		cfg := DefaultStatusConfig()
		if !isEmptyJSON(raw) {
			cfg = StatusConfig{}
			if err := decodeStrict(raw, &cfg); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
			}
		}
		if err := validateStatusConfig(cfg); err != nil {
			return nil, err
		}
		return json.Marshal(cfg)
	case TypeNumber:
		var cfg NumberConfig
		if !isEmptyJSON(raw) {
			if err := decodeStrict(raw, &cfg); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
			}
		}
		if utf8.RuneCountInString(cfg.Satuan) > 20 {
			return nil, fmt.Errorf("%w: satuan maksimal 20 karakter", ErrInvalidConfig)
		}
		return json.Marshal(cfg)
	default:
		if !isEmptyJSON(raw) && strings.TrimSpace(string(raw)) != "{}" {
			return nil, fmt.Errorf("%w: tipe %s tidak memiliki konfigurasi", ErrInvalidConfig, t)
		}
		return json.RawMessage(`{}`), nil
	}
}

func validateStatusConfig(cfg StatusConfig) error {
	if len(cfg.Labels) == 0 || len(cfg.Labels) > 50 {
		return fmt.Errorf("%w: label status 1 sampai 50", ErrInvalidConfig)
	}
	seen := map[string]bool{}
	for _, l := range cfg.Labels {
		switch {
		case !labelIDPattern.MatchString(l.ID):
			return fmt.Errorf("%w: id label %q harus huruf kecil, angka, atau _", ErrInvalidConfig, l.ID)
		case seen[l.ID]:
			return fmt.Errorf("%w: id label %q ganda", ErrInvalidConfig, l.ID)
		case strings.TrimSpace(l.Judul) == "" || utf8.RuneCountInString(l.Judul) > 50:
			return fmt.Errorf("%w: judul label 1 sampai 50 karakter", ErrInvalidConfig)
		case !ValidColor(l.Warna):
			return fmt.Errorf("%w: %w %q", ErrInvalidConfig, ErrInvalidColor, l.Warna)
		}
		seen[l.ID] = true
	}
	return nil
}

// statusLabelIDs mengembalikan id label dari konfigurasi status yang tersimpan.
func statusLabelIDs(config []byte) ([]string, error) {
	var cfg StatusConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("baca konfigurasi status: %w", err)
	}
	ids := make([]string, len(cfg.Labels))
	for i, l := range cfg.Labels {
		ids[i] = l.ID
	}
	return ids, nil
}

// Timeline adalah nilai kolom timeline.
type Timeline struct {
	Mulai   string `json:"mulai"`
	Selesai string `json:"selesai"`
}

var priorities = []string{"rendah", "sedang", "tinggi", "kritis"}

const (
	maxTextLen    = 2000
	maxPersonSize = 20
	dateLayout    = "2006-01-02"
)

// ValueContext memberi data yang dibutuhkan validator untuk tipe tertentu.
type ValueContext struct {
	// Config adalah konfigurasi kolom yang tersimpan (kanonis).
	Config []byte
	// IsMember memeriksa apakah pengguna adalah anggota proyek (kolom person).
	IsMember func(uuid.UUID) bool
}

// NormalizeValue memvalidasi nilai sel dan mengembalikan JSON kanonisnya.
// Nilai null tidak sampai ke sini; pemanggil menghapus sel.
func NormalizeValue(t ColumnType, raw json.RawMessage, vc ValueContext) (json.RawMessage, error) {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidValue, fmt.Sprintf(format, a...))
	}
	switch t {
	case TypeText:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, bad("teks harus string")
		}
		if utf8.RuneCountInString(s) > maxTextLen {
			return nil, bad("teks maksimal %d karakter", maxTextLen)
		}
		return json.Marshal(s)

	case TypeStatus:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, bad("status harus id label")
		}
		ids, err := statusLabelIDs(vc.Config)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(ids, s) {
			return nil, bad("label status %q tidak ada di kolom ini", s)
		}
		return json.Marshal(s)

	case TypePriority:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil || !slices.Contains(priorities, s) {
			return nil, bad("prioritas harus rendah, sedang, tinggi, atau kritis")
		}
		return json.Marshal(s)

	case TypePerson:
		var ids []uuid.UUID
		if err := json.Unmarshal(raw, &ids); err != nil {
			return nil, bad("person harus array UUID pengguna")
		}
		if len(ids) > maxPersonSize {
			return nil, bad("maksimal %d orang", maxPersonSize)
		}
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			// ASSUMPTION: hanya anggota proyek yang dapat ditugaskan pada item.
			if vc.IsMember == nil || !vc.IsMember(id) {
				return nil, bad("pengguna %s bukan anggota proyek", id)
			}
			if s := id.String(); !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
		return json.Marshal(out)

	case TypeDate:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, bad("tanggal harus string YYYY-MM-DD")
		}
		if _, err := time.Parse(dateLayout, s); err != nil {
			return nil, bad("tanggal harus berformat YYYY-MM-DD")
		}
		return json.Marshal(s)

	case TypeTimeline:
		var tl Timeline
		if err := decodeStrict(raw, &tl); err != nil {
			return nil, bad("timeline harus {\"mulai\", \"selesai\"}")
		}
		m, err1 := time.Parse(dateLayout, tl.Mulai)
		s, err2 := time.Parse(dateLayout, tl.Selesai)
		if err1 != nil || err2 != nil {
			return nil, bad("tanggal timeline harus berformat YYYY-MM-DD")
		}
		if s.Before(m) {
			return nil, bad("selesai tidak boleh sebelum mulai")
		}
		return json.Marshal(tl)

	case TypeNumber:
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, bad("angka tidak valid")
		}
		if math.Abs(f) > 1e15 {
			return nil, bad("angka di luar rentang")
		}
		return json.Marshal(f)

	case TypeProgress:
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil || f != math.Trunc(f) || f < 0 || f > 100 {
			return nil, bad("progres harus bilangan bulat 0 sampai 100")
		}
		return json.Marshal(int(f))

	case TypeDependency:
		return nil, ErrReadOnlyColumn
	}
	return nil, ErrInvalidColumnType
}

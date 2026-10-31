// Package automation menjalankan aturan otomasi no-code (SPEC M7):
// pemicu -> kondisi -> aksi, dengan antrean kejadian yang diisi trigger
// database dan dijalankan worker di latar belakang.
package automation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
)

// Jenis pemicu.
const (
	TriggerItemCreated    = "item_dibuat"
	TriggerStatusChanged  = "status_berubah"
	TriggerAssigneeChange = "penugasan_berubah"
	TriggerDateArrived    = "tanggal_tiba"
)

// Jenis kondisi.
const (
	CondColumnEquals = "kolom_sama"
	CondPersonHas    = "person_termasuk"
	CondInGroup      = "di_grup"
)

// Jenis aksi. Tidak ada aksi email (keputusan revisi M7).
const (
	ActionMoveGroup = "pindah_grup"
	ActionSetValue  = "ubah_kolom"
	ActionWebhook   = "panggil_webhook"
)

// Trigger adalah pemicu aturan.
type Trigger struct {
	Jenis string `json:"jenis"`
	// ColumnID: kolom status/person (opsional; kosong = kolom mana pun), atau
	// kolom date/timeline untuk tanggal tiba (wajib).
	ColumnID *uuid.UUID `json:"column_id,omitempty"`
	// Dari dan Ke: id label status (opsional) untuk status berubah.
	Dari *string `json:"dari,omitempty"`
	Ke   *string `json:"ke,omitempty"`
}

// Condition adalah satu syarat; semua syarat harus terpenuhi (AND).
type Condition struct {
	Jenis    string          `json:"jenis"`
	ColumnID *uuid.UUID      `json:"column_id,omitempty"`
	Nilai    json.RawMessage `json:"nilai,omitempty"`
	UserID   *uuid.UUID      `json:"user_id,omitempty"`
	GroupID  *uuid.UUID      `json:"group_id,omitempty"`
}

// Action adalah satu aksi.
type Action struct {
	Jenis    string          `json:"jenis"`
	GroupID  *uuid.UUID      `json:"group_id,omitempty"`
	ColumnID *uuid.UUID      `json:"column_id,omitempty"`
	Nilai    json.RawMessage `json:"nilai,omitempty"`
	// URL tujuan aksi panggil_webhook (https, host di daftar izin).
	URL string `json:"url,omitempty"`
}

// Event adalah satu kejadian dari antrean.
type Event struct {
	ID       int64
	BoardID  uuid.UUID
	ItemID   uuid.UUID
	Jenis    string
	ColumnID *uuid.UUID
	// RuleID: kejadian khusus satu aturan (tanggal tiba).
	RuleID  *uuid.UUID
	Sebelum json.RawMessage
	Sesudah json.RawMessage
	// Rantai: aturan yang (berurutan) menyebabkan kejadian ini.
	Rantai []uuid.UUID
}

// ItemState adalah keadaan item saat aturan dievaluasi.
type ItemState struct {
	GroupID uuid.UUID
	Values  map[uuid.UUID]json.RawMessage
}

func label(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

// Matches melaporkan apakah kejadian memicu aturan ruleID dengan pemicu t.
func (t Trigger) Matches(e Event, ruleID uuid.UUID) bool {
	if e.Jenis != t.Jenis {
		return false
	}
	if e.RuleID != nil && *e.RuleID != ruleID {
		return false
	}
	switch t.Jenis {
	case TriggerItemCreated:
		return true
	case TriggerStatusChanged:
		if t.ColumnID != nil && (e.ColumnID == nil || *e.ColumnID != *t.ColumnID) {
			return false
		}
		if t.Dari != nil && label(e.Sebelum) != *t.Dari {
			return false
		}
		return t.Ke == nil || label(e.Sesudah) == *t.Ke
	case TriggerAssigneeChange:
		return t.ColumnID == nil || (e.ColumnID != nil && *e.ColumnID == *t.ColumnID)
	case TriggerDateArrived:
		// Kejadian tanggal tiba selalu dibuat untuk satu aturan tertentu.
		return e.RuleID != nil
	}
	return false
}

// sameJSON membandingkan dua nilai JSON secara semantik; null dan kosong sama.
func sameJSON(a, b json.RawMessage) bool {
	a, b = bytes.TrimSpace(a), bytes.TrimSpace(b)
	if len(a) == 0 || string(a) == "null" {
		return len(b) == 0 || string(b) == "null"
	}
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	return bytes.Equal(xa, ya)
}

// ConditionsMet memeriksa semua kondisi; alasan berisi syarat pertama yang gagal.
func ConditionsMet(conds []Condition, it ItemState) (bool, string) {
	for i, c := range conds {
		switch c.Jenis {
		case CondColumnEquals:
			if c.ColumnID == nil || !sameJSON(it.Values[*c.ColumnID], c.Nilai) {
				return false, fmt.Sprintf("kondisi %d (nilai kolom) tidak terpenuhi", i+1)
			}
		case CondPersonHas:
			var ids []string
			if c.ColumnID != nil {
				_ = json.Unmarshal(it.Values[*c.ColumnID], &ids)
			}
			if c.UserID == nil || !slices.Contains(ids, c.UserID.String()) {
				return false, fmt.Sprintf("kondisi %d (penanggung jawab) tidak terpenuhi", i+1)
			}
		case CondInGroup:
			if c.GroupID == nil || *c.GroupID != it.GroupID {
				return false, fmt.Sprintf("kondisi %d (grup) tidak terpenuhi", i+1)
			}
		default:
			return false, fmt.Sprintf("kondisi %d tidak dikenal", i+1)
		}
	}
	return true, ""
}

// Limits adalah batas pencegahan loop.
type Limits struct {
	// MaxPerRule: satu aturan paling banyak berjalan N kali per item dalam satu rantai.
	MaxPerRule int
	// MaxDepth: panjang rantai sebab-akibat paling panjang.
	MaxDepth int
}

// DefaultLimits sesuai keputusan M7.
func DefaultLimits() Limits { return Limits{MaxPerRule: 3, MaxDepth: 5} }

// LoopBlocked melaporkan apakah aturan tidak boleh berjalan lagi pada kejadian
// dengan rantai ini. Rantai kejadian dari aturan R berisi R di ujungnya, jadi
// kemunculan R di rantai = berapa kali R sudah berjalan dalam rantai ini.
func (l Limits) LoopBlocked(rantai []uuid.UUID, rule uuid.UUID) (bool, string) {
	n := 0
	for _, r := range rantai {
		if r == rule {
			n++
		}
	}
	if n >= l.MaxPerRule {
		return true, fmt.Sprintf("aturan sudah berjalan %d kali dalam satu rantai (batas %d)", n, l.MaxPerRule)
	}
	if len(rantai) >= l.MaxDepth {
		return true, fmt.Sprintf("rantai otomasi sudah %d langkah (batas %d)", len(rantai), l.MaxDepth)
	}
	return false, ""
}

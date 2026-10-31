package automation

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func ptr[T any](v T) *T { return &v }

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func TestTriggerMatches(t *testing.T) {
	rule, other := uuid.New(), uuid.New()
	colA, colB := uuid.New(), uuid.New()
	status := func(dari, ke string) Event {
		e := Event{Jenis: TriggerStatusChanged, ColumnID: &colA, Sesudah: raw(`"` + ke + `"`)}
		if dari != "" {
			e.Sebelum = raw(`"` + dari + `"`)
		}
		return e
	}
	tests := []struct {
		name string
		t    Trigger
		e    Event
		want bool
	}{
		{"item dibuat", Trigger{Jenis: TriggerItemCreated}, Event{Jenis: TriggerItemCreated}, true},
		{"jenis berbeda", Trigger{Jenis: TriggerItemCreated}, status("", "done"), false},
		{"status apa pun", Trigger{Jenis: TriggerStatusChanged}, status("waiting", "done"), true},
		{"status ke done", Trigger{Jenis: TriggerStatusChanged, Ke: ptr("done")}, status("waiting", "done"), true},
		{"status ke label lain", Trigger{Jenis: TriggerStatusChanged, Ke: ptr("done")}, status("done", "waiting"), false},
		{"dari waiting ke done", Trigger{Jenis: TriggerStatusChanged, Dari: ptr("waiting"), Ke: ptr("done")}, status("waiting", "done"), true},
		{"dari label lain", Trigger{Jenis: TriggerStatusChanged, Dari: ptr("waiting")}, status("in_progress", "done"), false},
		{"dari kosong (item baru diberi status)", Trigger{Jenis: TriggerStatusChanged, Dari: ptr("")}, status("", "done"), true},
		{"status dihapus: ke kosong", Trigger{Jenis: TriggerStatusChanged, Ke: ptr("")}, Event{Jenis: TriggerStatusChanged, ColumnID: &colA, Sebelum: raw(`"done"`)}, true},
		{"kolom status sesuai", Trigger{Jenis: TriggerStatusChanged, ColumnID: &colA}, status("", "done"), true},
		{"kolom status lain", Trigger{Jenis: TriggerStatusChanged, ColumnID: &colB}, status("", "done"), false},
		{"penugasan", Trigger{Jenis: TriggerAssigneeChange}, Event{Jenis: TriggerAssigneeChange, ColumnID: &colB}, true},
		{"penugasan kolom lain", Trigger{Jenis: TriggerAssigneeChange, ColumnID: &colA}, Event{Jenis: TriggerAssigneeChange, ColumnID: &colB}, false},
		{"tanggal tiba untuk aturan ini", Trigger{Jenis: TriggerDateArrived, ColumnID: &colA}, Event{Jenis: TriggerDateArrived, RuleID: &rule}, true},
		{"tanggal tiba untuk aturan lain", Trigger{Jenis: TriggerDateArrived, ColumnID: &colA}, Event{Jenis: TriggerDateArrived, RuleID: &other}, false},
		{"tanggal tiba tanpa aturan", Trigger{Jenis: TriggerDateArrived}, Event{Jenis: TriggerDateArrived}, false},
		{"pemicu tidak dikenal", Trigger{Jenis: "lain"}, Event{Jenis: "lain"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.t.Matches(tt.e, rule))
		})
	}
}

func TestConditionsMet(t *testing.T) {
	status, person, prio := uuid.New(), uuid.New(), uuid.New()
	g1, g2 := uuid.New(), uuid.New()
	u1, u2 := uuid.New(), uuid.New()
	it := ItemState{GroupID: g1, Values: map[uuid.UUID]json.RawMessage{
		status: raw(`"done"`),
		person: raw(`["` + u1.String() + `"]`),
	}}
	tests := []struct {
		name  string
		conds []Condition
		ok    bool
	}{
		{"tanpa kondisi", nil, true},
		{"status sama", []Condition{{Jenis: CondColumnEquals, ColumnID: &status, Nilai: raw(` "done" `)}}, true},
		{"status beda", []Condition{{Jenis: CondColumnEquals, ColumnID: &status, Nilai: raw(`"waiting"`)}}, false},
		{"kolom kosong = null", []Condition{{Jenis: CondColumnEquals, ColumnID: &prio, Nilai: raw(`null`)}}, true},
		{"kolom kosong bukan nilai", []Condition{{Jenis: CondColumnEquals, ColumnID: &prio, Nilai: raw(`"tinggi"`)}}, false},
		{"penanggung jawab termasuk", []Condition{{Jenis: CondPersonHas, ColumnID: &person, UserID: &u1}}, true},
		{"penanggung jawab lain", []Condition{{Jenis: CondPersonHas, ColumnID: &person, UserID: &u2}}, false},
		{"di grup", []Condition{{Jenis: CondInGroup, GroupID: &g1}}, true},
		{"grup lain", []Condition{{Jenis: CondInGroup, GroupID: &g2}}, false},
		{
			"AND: satu gagal",
			[]Condition{{Jenis: CondInGroup, GroupID: &g1}, {Jenis: CondColumnEquals, ColumnID: &status, Nilai: raw(`"waiting"`)}},
			false,
		},
		{"kondisi tidak dikenal", []Condition{{Jenis: "lain"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, reason := ConditionsMet(tt.conds, it)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.ok, reason == "", reason)
		})
	}
}

func TestLoopBlocked(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	l := DefaultLimits()
	tests := []struct {
		name   string
		rantai []uuid.UUID
		rule   uuid.UUID
		block  bool
	}{
		{"dari pengguna", nil, a, false},
		{"sekali", []uuid.UUID{a}, a, false},
		{"dua kali", []uuid.UUID{a, b, a}, a, false},
		{"tiga kali: diblokir", []uuid.UUID{a, b, a, b, a}, a, true},
		{"aturan lain pada rantai yang sama", []uuid.UUID{a, a, a}, b, false},
		{"kedalaman 5: diblokir", []uuid.UUID{a, b, a, b, uuid.New()}, uuid.New(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocked, reason := l.LoopBlocked(tt.rantai, tt.rule)
			assert.Equal(t, tt.block, blocked)
			assert.Equal(t, tt.block, reason != "")
		})
	}
}

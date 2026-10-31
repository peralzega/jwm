package report

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jwm/internal/schedule"
)

func day(t *testing.T, s string) int {
	t.Helper()
	d, err := schedule.ParseDay(s)
	require.NoError(t, err)
	return d
}

func cal(t *testing.T, libur ...string) *schedule.Calendar {
	t.Helper()
	var days []schedule.CalendarDay
	for _, d := range libur {
		days = append(days, schedule.CalendarDay{Tanggal: d, Keterangan: "Libur"})
	}
	c, err := schedule.NewCalendar(days)
	require.NoError(t, err)
	return c
}

func uid(name string) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte(name)) }

// item membuat ItemFacts; mulai kosong berarti tanpa jadwal.
func item(t *testing.T, name, mulai, selesai string, opts ...func(*ItemFacts)) ItemFacts {
	t.Helper()
	it := ItemFacts{ID: uid(name), Judul: name, HasAssigneeCol: true, Assigned: true}
	if mulai != "" {
		it.Span = &schedule.Span{Start: day(t, mulai), End: day(t, selesai)}
	}
	for _, o := range opts {
		o(&it)
	}
	return it
}

func done(it *ItemFacts)                  { it.Done = true; it.Progress = 100 }
func progress(p float64) func(*ItemFacts) { return func(it *ItemFacts) { it.Progress = p } }
func unassigned(it *ItemFacts)            { it.Assigned = false }
func noPersonCol(it *ItemFacts)           { it.HasAssigneeCol = false; it.Assigned = false }
func critical(ef string, late bool) func(*ItemFacts) {
	return func(it *ItemFacts) {
		e := ef
		it.Sched = &schedule.ItemSchedule{ItemID: it.ID, EF: &e, Kritis: true, MelanggarFS: late}
	}
}
func notCritical(it *ItemFacts) { it.Sched = &schedule.ItemSchedule{ItemID: it.ID, Kritis: false} }
func preds(names ...string) func(*ItemFacts) {
	return func(it *ItemFacts) {
		for _, n := range names {
			it.Preds = append(it.Preds, uid(n))
		}
	}
}

// Hari ini: Kamis 5 November 2026.
const today = "2026-11-05"

func TestRiskRules(t *testing.T) {
	tests := []struct {
		name     string
		libur    []string
		deadline string
		items    func(t *testing.T) []ItemFacts
		want     map[string][]string // judul -> kode aturan
	}{
		{
			name: "item Done tidak pernah berisiko",
			items: func(t *testing.T) []ItemFacts {
				return []ItemFacts{item(t, "A", "2026-11-02", "2026-11-03", done, unassigned)}
			},
			want: map[string][]string{},
		},
		{
			name: "aturan 1: lewat tenggat",
			items: func(t *testing.T) []ItemFacts {
				return []ItemFacts{
					item(t, "lewat", "2026-11-02", "2026-11-04", progress(90)),
					item(t, "hari ini", "2026-11-02", "2026-11-05", progress(90)),
				}
			},
			want: map[string][]string{"lewat": {RuleOverdue}},
		},
		{
			name: "aturan 2: tenggat ≤ 3 hari kerja dan progres < 50%",
			items: func(t *testing.T) []ItemFacts {
				return []ItemFacts{
					item(t, "Senin 40%", "2026-11-02", "2026-11-09", progress(40)), // sisa Jumat, Senin = 2
					item(t, "Senin 50%", "2026-11-02", "2026-11-09", progress(50)),
					item(t, "hari ini 0%", "2026-11-02", "2026-11-05"),
					item(t, "Selasa 0%", "2026-11-02", "2026-11-10"), // sisa 3
					item(t, "Rabu 0%", "2026-11-02", "2026-11-11"),   // sisa 4
					item(t, "tanpa jadwal", "", ""),
				}
			},
			want: map[string][]string{
				"Senin 40%": {RuleDueSoon}, "hari ini 0%": {RuleDueSoon}, "Selasa 0%": {RuleDueSoon},
			},
		},
		{
			name:  "aturan 2: hari libur tidak dihitung dalam sisa hari kerja",
			libur: []string{"2026-11-09"},
			items: func(t *testing.T) []ItemFacts {
				return []ItemFacts{item(t, "Rabu 0%", "2026-11-02", "2026-11-11")} // sisa Jumat, Selasa, Rabu = 3
			},
			want: map[string][]string{"Rabu 0%": {RuleDueSoon}},
		},
		{
			name: "aturan 3: tanpa penanggung jawab, hanya bila board punya kolom person",
			items: func(t *testing.T) []ItemFacts {
				return []ItemFacts{
					item(t, "kosong", "", "", unassigned),
					item(t, "tanpa kolom", "", "", noPersonCol),
				}
			},
			want: map[string][]string{"kosong": {RuleNoAssignee}},
		},
		{
			name: "aturan 4: predecessor terlambat pada item kritis",
			items: func(t *testing.T) []ItemFacts {
				return []ItemFacts{
					item(t, "P", "2026-11-02", "2026-11-03", progress(80), notCritical),
					item(t, "kritis", "2026-11-06", "2026-11-20", preds("P"), critical("2026-11-20", false)),
					item(t, "tidak kritis", "2026-11-06", "2026-11-20", preds("P"), notCritical),
					item(t, "melanggar", "2026-11-06", "2026-11-20", critical("2026-11-20", true)),
					item(t, "P selesai", "2026-11-02", "2026-11-03", done),
					item(t, "pred Done", "2026-11-06", "2026-11-20", preds("P selesai"), critical("2026-11-20", false)),
				}
			},
			want: map[string][]string{
				"P": {RuleOverdue}, "kritis": {RuleLatePredecessor}, "melanggar": {RuleLatePredecessor},
			},
		},
		{
			name:     "aturan 5: slack ≤ 0 terhadap tenggat proyek pada jalur kritis",
			deadline: "2026-11-20",
			items: func(t *testing.T) []ItemFacts {
				return []ItemFacts{
					item(t, "tepat", "2026-11-06", "2026-11-20", critical("2026-11-20", false)),
					item(t, "lewat", "2026-11-06", "2026-11-24", critical("2026-11-24", false)),
					item(t, "longgar", "2026-11-06", "2026-11-18", critical("2026-11-18", false)),
					item(t, "tidak kritis", "2026-11-06", "2026-11-24", notCritical),
				}
			},
			want: map[string][]string{"tepat": {RuleProjectDeadline}, "lewat": {RuleProjectDeadline}},
		},
		{
			name: "aturan 5 dilewati bila proyek tanpa tanggal selesai",
			items: func(t *testing.T) []ItemFacts {
				return []ItemFacts{item(t, "lewat", "2026-11-06", "2026-11-24", critical("2026-11-24", false))}
			},
			want: map[string][]string{},
		},
		{
			name: "beberapa aturan pada satu item",
			items: func(t *testing.T) []ItemFacts {
				return []ItemFacts{item(t, "semua", "2026-11-02", "2026-11-04", unassigned)}
			},
			want: map[string][]string{"semua": {RuleOverdue, RuleNoAssignee}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := ProjectInput{Items: tt.items(t), Today: day(t, today), Cal: cal(t, tt.libur...)}
			if tt.deadline != "" {
				d := day(t, tt.deadline)
				in.Deadline = &d
			}
			res := Evaluate(in, DefaultThresholds())
			got := map[string][]string{}
			perRule := map[string]int{}
			for _, ir := range res.Items {
				for _, r := range ir.Alasan {
					got[ir.Judul] = append(got[ir.Judul], r.Kode)
					perRule[r.Kode]++
					assert.NotEmpty(t, r.Pesan)
				}
			}
			assert.Equal(t, tt.want, got)
			assert.Equal(t, len(tt.want), res.ItemBerisiko)
			for _, r := range Rules {
				assert.Equal(t, perRule[r], res.PerAturan[r], r)
			}
		})
	}
}

func TestRiskMessagesAreTraceable(t *testing.T) {
	d := day(t, "2026-11-18")
	res := Evaluate(ProjectInput{
		Items: []ItemFacts{
			item(t, "Desain", "2026-11-02", "2026-11-03", notCritical),
			item(t, "Bangun", "2026-11-06", "2026-11-20", preds("Desain"), critical("2026-11-20", false)),
		},
		Deadline: &d, Today: day(t, today), Cal: cal(t),
	}, DefaultThresholds())
	require.Len(t, res.Items, 2)
	msgs := fmt.Sprint(res.Items[1].Alasan)
	assert.Contains(t, msgs, `predecessor terlambat: "Desain"`)
	assert.Contains(t, msgs, "slack -2 hari kerja terhadap tenggat proyek 2026-11-18")
	assert.Contains(t, fmt.Sprint(res.Items[0].Alasan), "Lewat tenggat 2026-11-03")
}

func TestHealthAndProgress(t *testing.T) {
	// n item berjadwal jauh ke depan (tidak berisiko), ditambah r item tanpa penanggung jawab.
	batch := func(t *testing.T, n, r int) []ItemFacts {
		var out []ItemFacts
		for i := range n {
			opts := []func(*ItemFacts){progress(0)}
			if i < r {
				opts = append(opts, unassigned)
			}
			out = append(out, item(t, fmt.Sprintf("i%d", i), "2026-12-01", "2026-12-31", opts...))
		}
		return out
	}
	tests := []struct {
		name    string
		items   func(t *testing.T) []ItemFacts
		health  string
		progres *float64
		rencana *float64
		alasan  string
	}{
		{name: "tanpa item", items: func(*testing.T) []ItemFacts { return nil }, health: Green, alasan: "Belum ada item"},
		{name: "0% berisiko", items: func(t *testing.T) []ItemFacts { return batch(t, 10, 0) }, health: Green},
		{name: "9% berisiko", items: func(t *testing.T) []ItemFacts { return batch(t, 11, 1) }, health: Green},
		{name: "10% berisiko", items: func(t *testing.T) []ItemFacts { return batch(t, 10, 1) }, health: Yellow, alasan: "10% item berisiko"},
		{name: "25% berisiko", items: func(t *testing.T) []ItemFacts { return batch(t, 20, 5) }, health: Yellow},
		{name: "30% berisiko", items: func(t *testing.T) []ItemFacts { return batch(t, 10, 3) }, health: Red, alasan: "Merah bila > 25%"},
		{
			name: "satu item kritis terlambat langsung Merah",
			items: func(t *testing.T) []ItemFacts {
				return append(batch(t, 19, 0), item(t, "telat", "2026-11-02", "2026-11-04", critical("2026-11-04", false)))
			},
			health: Red, alasan: "1 item di jalur kritis terlambat",
		},
		{
			// 2–6 Nov (5 hari kerja), hari ini Kamis: rencana 4/5 = 80%.
			name: "tertinggal > 10 poin dari rencana: Kuning",
			items: func(t *testing.T) []ItemFacts {
				return []ItemFacts{item(t, "A", "2026-11-02", "2026-11-06", progress(60))}
			},
			health:  Yellow,
			progres: ptr(60), rencana: ptr(80), alasan: "tertinggal dari rencana 80%",
		},
		{
			name: "tertinggal ≤ 10 poin: Hijau",
			items: func(t *testing.T) []ItemFacts {
				return []ItemFacts{item(t, "A", "2026-11-02", "2026-11-06", progress(75))}
			},
			health:  Green,
			progres: ptr(75), rencana: ptr(80),
		},
		{
			// Bobot durasi: A 10 hari kerja 100%, B tanpa jadwal (bobot 1) 0%.
			name: "progres tertimbang durasi hari kerja",
			items: func(t *testing.T) []ItemFacts {
				return []ItemFacts{
					item(t, "A", "2026-10-19", "2026-10-30", done),
					item(t, "B", "", ""),
				}
			},
			health:  Green,
			progres: ptr(90.9), rencana: ptr(100),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Evaluate(ProjectInput{Items: tt.items(t), Today: day(t, today), Cal: cal(t)}, DefaultThresholds())
			assert.Equal(t, tt.health, res.Health, res.Alasan)
			if tt.progres != nil {
				require.NotNil(t, res.Progres)
				assert.InDelta(t, *tt.progres, *res.Progres, 0.05)
			}
			if tt.rencana != nil {
				require.NotNil(t, res.ProgresRencana)
				assert.InDelta(t, *tt.rencana, *res.ProgresRencana, 0.05)
			}
			if tt.alasan != "" {
				assert.Contains(t, fmt.Sprint(res.Alasan), tt.alasan)
			}
			assert.NotEmpty(t, res.Alasan)
		})
	}
}

func TestThresholdsAreConfigurable(t *testing.T) {
	items := []ItemFacts{item(t, "A", "2026-11-02", "2026-11-11", progress(70))} // sisa 4 hari kerja
	th := DefaultThresholds()
	assert.Equal(t, 0, Evaluate(ProjectInput{Items: items, Today: day(t, today), Cal: cal(t)}, th).ItemBerisiko)
	th.DueDays, th.DueProgress = 5, 80
	assert.Equal(t, 1, Evaluate(ProjectInput{Items: items, Today: day(t, today), Cal: cal(t)}, th).ItemBerisiko)
}

func ptr(f float64) *float64 { return &f }

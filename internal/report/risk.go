// Package report menghitung progres, risiko item, dan health proyek
// (SPEC bagian 5 dan M5), serta kelak status reporter (M8).
package report

import (
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"

	"jwm/internal/schedule"
)

// Thresholds adalah ambang aturan risiko dan health.
// ASSUMPTION: nilai bawaan dari SPEC bagian 5; dapat diatur lewat env, belum ada UI.
type Thresholds struct {
	// DueDays: tenggat dalam N hari kerja dianggap dekat (aturan 2).
	DueDays int `json:"tenggat_dekat_hari"`
	// DueProgress: progres di bawah nilai ini dengan tenggat dekat berisiko (aturan 2).
	DueProgress int `json:"tenggat_dekat_progres"`
	// YellowPct dan RedPct: persentase item berisiko untuk Kuning (>=) dan Merah (>).
	YellowPct int `json:"kuning_persen"`
	RedPct    int `json:"merah_persen"`
	// BehindPoints: progres tertinggal lebih dari N poin dari rencana membuat Kuning.
	BehindPoints int `json:"tertinggal_poin"`
}

// DefaultThresholds sesuai SPEC bagian 5.
func DefaultThresholds() Thresholds {
	return Thresholds{DueDays: 3, DueProgress: 50, YellowPct: 10, RedPct: 25, BehindPoints: 10}
}

// Kode aturan risiko item (SPEC bagian 5, urutan sama).
const (
	RuleOverdue         = "lewat_tenggat"
	RuleDueSoon         = "tenggat_dekat"
	RuleNoAssignee      = "tanpa_penanggung_jawab"
	RuleLatePredecessor = "predecessor_terlambat"
	RuleProjectDeadline = "melewati_tenggat_proyek"
)

// Rules adalah semua kode aturan dalam urutan SPEC.
var Rules = []string{RuleOverdue, RuleDueSoon, RuleNoAssignee, RuleLatePredecessor, RuleProjectDeadline}

// Health proyek.
const (
	Green  = "hijau"
	Yellow = "kuning"
	Red    = "merah"
)

// ItemFacts adalah data satu item yang dibutuhkan aturan risiko.
type ItemFacts struct {
	ID      uuid.UUID
	BoardID uuid.UUID
	Judul   string
	// Done: status pada kolom status pertama berlabel "done".
	Done bool
	// Progress 0–100: kolom progress pertama; bila kosong, 100 untuk Done dan 0 selainnya.
	Progress float64
	// HasAssigneeCol: board punya kolom person; Assigned: kolom person pertama terisi.
	HasAssigneeCol bool
	Assigned       bool
	// Span: timeline utama (nil bila tanpa jadwal).
	Span *schedule.Span
	// Sched: hasil CPM item (nil bila tidak ikut dihitung).
	Sched *schedule.ItemSchedule
	// Preds: predecessor langsung.
	Preds []uuid.UUID
}

// Reason adalah satu alasan risiko yang dapat ditelusuri.
type Reason struct {
	Kode  string `json:"kode"`
	Pesan string `json:"pesan"`
}

// ItemRisk adalah item berisiko beserta alasannya.
type ItemRisk struct {
	ItemID  uuid.UUID `json:"item_id"`
	BoardID uuid.UUID `json:"board_id"`
	Judul   string    `json:"judul"`
	Kritis  bool      `json:"kritis"`
	Alasan  []Reason  `json:"alasan"`
}

// ProjectInput adalah masukan evaluasi satu proyek.
type ProjectInput struct {
	Items []ItemFacts
	// Deadline: tanggal selesai proyek (nomor hari); nil bila belum diisi.
	Deadline *int
	Today    int
	Cal      *schedule.Calendar
}

// ProjectResult adalah hasil evaluasi satu proyek.
type ProjectResult struct {
	// Progres: rata-rata tertimbang (bobot = durasi hari kerja timeline utama,
	// minimal 1; item tanpa timeline berbobot 1). nil bila tanpa item.
	Progres *float64
	// ProgresRencana: porsi hari kerja timeline yang sudah lewat, ditimbang sama,
	// hanya untuk item berjadwal. ProgresJadwal adalah progres aktual pada item yang sama.
	ProgresRencana  *float64
	ProgresJadwal   *float64
	JumlahItem      int
	ItemBerisiko    int
	PerAturan       map[string]int
	KritisTerlambat int
	Health          string
	Alasan          []string
	Items           []ItemRisk
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// Evaluate menerapkan aturan risiko pada setiap item lalu menentukan health.
// Item Done tidak pernah berisiko.
func Evaluate(in ProjectInput, th Thresholds) ProjectResult {
	res := ProjectResult{JumlahItem: len(in.Items), PerAturan: map[string]int{}, Health: Green}
	for _, r := range Rules {
		res.PerAturan[r] = 0
	}
	byID := make(map[uuid.UUID]*ItemFacts, len(in.Items))
	for i := range in.Items {
		byID[in.Items[i].ID] = &in.Items[i]
	}
	overdue := func(it *ItemFacts) bool { return !it.Done && it.Span != nil && it.Span.End < in.Today }

	var wSum, pSum, wSched, planSum, actSched float64
	for i := range in.Items {
		it := &in.Items[i]
		dur := 0
		if it.Span != nil {
			dur = in.Cal.WorkdaysIn(it.Span.Start, it.Span.End+1)
		}
		w := float64(max(dur, 1))
		wSum += w
		pSum += w * it.Progress
		if it.Span != nil {
			var plan float64
			switch {
			case in.Today < it.Span.Start:
				plan = 0
			case in.Today >= it.Span.End:
				plan = 100
			case dur == 0:
				plan = 0
			default:
				plan = 100 * float64(in.Cal.WorkdaysIn(it.Span.Start, in.Today+1)) / float64(dur)
			}
			wSched += w
			planSum += w * plan
			actSched += w * it.Progress
		}

		if it.Done {
			continue
		}
		kritis := it.Sched != nil && it.Sched.Kritis
		var reasons []Reason
		if overdue(it) {
			reasons = append(reasons, Reason{RuleOverdue,
				fmt.Sprintf("Lewat tenggat %s dan belum Done.", schedule.FormatDay(it.Span.End))})
			if kritis {
				res.KritisTerlambat++
			}
		} else if it.Span != nil && it.Span.End >= in.Today {
			left := in.Cal.WorkdaysIn(in.Today+1, it.Span.End+1)
			if left <= th.DueDays && it.Progress < float64(th.DueProgress) {
				reasons = append(reasons, Reason{RuleDueSoon, fmt.Sprintf(
					"Tenggat %s tinggal %d hari kerja, progres %.0f%% (< %d%%).",
					schedule.FormatDay(it.Span.End), left, it.Progress, th.DueProgress)})
			}
		}
		if it.HasAssigneeCol && !it.Assigned {
			reasons = append(reasons, Reason{RuleNoAssignee, "Belum ada penanggung jawab."})
		}
		if kritis {
			var late []string
			for _, p := range it.Preds {
				if pi := byID[p]; pi != nil && overdue(pi) {
					late = append(late, fmt.Sprintf("%q", pi.Judul))
				}
			}
			switch {
			case len(late) > 0:
				reasons = append(reasons, Reason{RuleLatePredecessor,
					fmt.Sprintf("Di jalur kritis dan predecessor terlambat: %s.", joinMax(late, 3))})
			case it.Sched.MelanggarFS:
				reasons = append(reasons, Reason{RuleLatePredecessor,
					"Di jalur kritis dan dijadwalkan mulai sebelum predecessor selesai."})
			}
			if in.Deadline != nil && it.Sched.EF != nil {
				if ef, err := schedule.ParseDay(*it.Sched.EF); err == nil {
					margin := in.Cal.WorkdaysIn(ef+1, *in.Deadline+1) - in.Cal.WorkdaysIn(*in.Deadline+1, ef+1)
					if margin <= 0 {
						reasons = append(reasons, Reason{RuleProjectDeadline, fmt.Sprintf(
							"Di jalur kritis; perkiraan selesai %s, slack %d hari kerja terhadap tenggat proyek %s.",
							*it.Sched.EF, margin, schedule.FormatDay(*in.Deadline))})
					}
				}
			}
		}
		if len(reasons) == 0 {
			continue
		}
		for _, r := range reasons {
			res.PerAturan[r.Kode]++
		}
		res.ItemBerisiko++
		res.Items = append(res.Items, ItemRisk{ItemID: it.ID, BoardID: it.BoardID, Judul: it.Judul, Kritis: kritis, Alasan: reasons})
	}

	if res.JumlahItem == 0 {
		res.Alasan = []string{"Belum ada item."}
		return res
	}
	prog := round1(pSum / wSum)
	res.Progres = &prog
	behind := false
	if wSched > 0 {
		plan, act := round1(planSum/wSched), round1(actSched/wSched)
		res.ProgresRencana, res.ProgresJadwal = &plan, &act
		behind = plan-act > float64(th.BehindPoints)
	}

	pct := 100 * float64(res.ItemBerisiko) / float64(res.JumlahItem)
	switch {
	case res.KritisTerlambat > 0 || pct > float64(th.RedPct):
		res.Health = Red
	case pct >= float64(th.YellowPct) || behind:
		res.Health = Yellow
	}
	if res.KritisTerlambat > 0 {
		res.Alasan = append(res.Alasan, fmt.Sprintf("%d item di jalur kritis terlambat.", res.KritisTerlambat))
	}
	switch {
	case pct > float64(th.RedPct):
		res.Alasan = append(res.Alasan, fmt.Sprintf("%.0f%% item berisiko (Merah bila > %d%%).", pct, th.RedPct))
	case pct >= float64(th.YellowPct):
		res.Alasan = append(res.Alasan, fmt.Sprintf("%.0f%% item berisiko (Kuning %d–%d%%).", pct, th.YellowPct, th.RedPct))
	}
	if behind {
		res.Alasan = append(res.Alasan, fmt.Sprintf("Progres %.0f%% tertinggal dari rencana %.0f%% (> %d poin).",
			*res.ProgresJadwal, *res.ProgresRencana, th.BehindPoints))
	}
	if len(res.Alasan) == 0 {
		res.Alasan = []string{fmt.Sprintf("%.0f%% item berisiko; progres sesuai rencana.", pct)}
	}
	return res
}

func joinMax(s []string, n int) string {
	if len(s) <= n {
		return strings.Join(s, ", ")
	}
	return fmt.Sprintf("%s, dan %d lainnya", strings.Join(s[:n], ", "), len(s)-n)
}

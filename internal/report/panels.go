package report

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"jwm/internal/auth"
	"jwm/internal/board"
	"jwm/internal/db/gen"
	"jwm/internal/project"
	"jwm/internal/schedule"
)

// TrendWeeks adalah panjang tren penyelesaian (minggu, termasuk minggu ini).
const TrendWeeks = 12

// StatusSlice adalah jumlah item untuk satu label status (digabung per judul label).
type StatusSlice struct {
	Label  string `json:"label"`
	Warna  string `json:"warna"`
	Jumlah int    `json:"jumlah"`
}

// Workload adalah beban kerja satu orang: item terbuka (bukan Done) yang
// menugaskannya. Item dengan beberapa penanggung jawab dihitung penuh untuk
// masing-masing (keputusan M6).
type Workload struct {
	UserID      uuid.UUID `json:"user_id"`
	Nama        string    `json:"nama"`
	ItemTerbuka int       `json:"item_terbuka"`
	// HariKerja: jumlah durasi hari kerja timeline utama item terbuka.
	HariKerja int `json:"hari_kerja"`
	Terlambat int `json:"terlambat"`
}

// WeekCount adalah jumlah item yang selesai dalam satu minggu (mulai Senin).
type WeekCount struct {
	MingguMulai string `json:"minggu_mulai"`
	Selesai     int    `json:"selesai"`
}

// Finance adalah ringkasan keuangan satu proyek.
type Finance struct {
	ProjectID   uuid.UUID `json:"project_id"`
	Nama        string    `json:"nama"`
	Anggaran    string    `json:"anggaran"`
	Realisasi   string    `json:"realisasi"`
	Sisa        string    `json:"sisa"`
	JumlahEntri int32     `json:"jumlah_entri"`
}

// Panels adalah respons GET /dashboard.
type Panels struct {
	// ProjectID nil berarti semua proyek aktif yang terlihat pelaku.
	ProjectID       *uuid.UUID    `json:"project_id"`
	HariIni         string        `json:"hari_ini"`
	JumlahItem      int           `json:"jumlah_item"`
	Status          []StatusSlice `json:"status"`
	TanpaStatus     int           `json:"tanpa_status"`
	Beban           []Workload    `json:"beban"`
	BelumDitugaskan int           `json:"belum_ditugaskan"`
	Tren            []WeekCount   `json:"tren"`
	Keuangan        []Finance     `json:"keuangan"`
}

// mondayOf memberi nomor hari Senin pada minggu yang memuat day.
func mondayOf(day int) int {
	// 1970-01-01 adalah Kamis; (day+3)%7 = 0 untuk Senin.
	return day - ((day+3)%7+7)%7
}

// Panels menghitung panel dasbor untuk satu proyek (projectID) atau semua
// proyek aktif yang terlihat pelaku (nil).
func (s *Service) Panels(ctx context.Context, actor auth.Principal, projectID *uuid.UUID) (Panels, error) {
	rows, err := s.visibleProjects(ctx, actor)
	if err != nil {
		return Panels{}, err
	}
	if projectID != nil {
		rows = slices.DeleteFunc(rows, func(r dbgen.ListDashboardProjectsRow) bool { return r.ID != *projectID })
		if len(rows) == 0 {
			// Tidak terlihat, tidak ada, atau diarsipkan.
			return Panels{}, project.ErrNotFound
		}
	}
	now := s.now()
	today := schedule.DayOf(now)
	out := Panels{
		ProjectID: projectID, HariIni: schedule.FormatDay(today),
		Status: []StatusSlice{}, Beban: []Workload{}, Tren: make([]WeekCount, TrendWeeks), Keuangan: []Finance{},
	}
	firstWeek := mondayOf(today) - 7*(TrendWeeks-1)
	for i := range out.Tren {
		out.Tren[i].MingguMulai = schedule.FormatDay(firstWeek + 7*i)
	}
	if len(rows) == 0 {
		return out, nil
	}

	projectIDs := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		projectIDs[i] = r.ID
	}
	q, done, err := s.readTx(ctx)
	if err != nil {
		return Panels{}, err
	}
	defer done()
	boards, err := q.ListBoardsOfProjects(ctx, projectIDs)
	if err != nil {
		return Panels{}, fmt.Errorf("daftar board: %w", err)
	}
	boardIDs := make([]uuid.UUID, len(boards))
	for i, b := range boards {
		boardIDs[i] = b.ID
	}
	cols, err := q.ListColumnsOfBoards(ctx, boardIDs)
	if err != nil {
		return Panels{}, fmt.Errorf("daftar kolom: %w", err)
	}
	pk := pickColumns(cols)
	params := pk.params
	params.BoardIds = boardIDs
	items, err := q.ListPortfolioItems(ctx, params)
	if err != nil {
		return Panels{}, fmt.Errorf("baca item: %w", err)
	}
	assignees, err := q.ListItemAssignees(ctx, dbgen.ListItemAssigneesParams{PersonCols: params.PersonCols, BoardIds: boardIDs})
	if err != nil {
		return Panels{}, fmt.Errorf("baca penanggung jawab: %w", err)
	}
	// Batas awal tren pada tengah malam waktu lokal server (lihat ASSUMPTION "hari ini").
	sejak := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, firstWeek-today)
	transitions, err := q.ListDoneTransitions(ctx, dbgen.ListDoneTransitionsParams{
		BoardIds: boardIDs, Sejak: sejak, StatusCols: params.StatusCols,
	})
	if err != nil {
		return Panels{}, fmt.Errorf("baca riwayat status: %w", err)
	}
	totals, err := q.BudgetTotals(ctx, projectIDs)
	if err != nil {
		return Panels{}, fmt.Errorf("total anggaran: %w", err)
	}
	cal, err := schedule.LoadCalendar(ctx, q)
	if err != nil {
		return Panels{}, err
	}

	// Label status per board: id -> (judul, warna) dari konfigurasi kolom status terpilih.
	type label struct{ judul, warna string }
	labels := map[uuid.UUID]map[string]label{}
	for _, c := range pk.status {
		var cfg board.StatusConfig
		_ = json.Unmarshal(c.KonfigurasiJson, &cfg)
		m := map[string]label{}
		for _, l := range cfg.Labels {
			m[l.ID] = label{l.Judul, l.Warna}
		}
		labels[c.BoardID] = m
	}

	// Sebaran status, digabung per judul label (tanpa membedakan huruf besar kecil).
	slot := map[string]int{}
	type openItem struct {
		span *schedule.Span
	}
	open := map[uuid.UUID]openItem{}
	doneNow := map[uuid.UUID]bool{}
	out.JumlahItem = len(items)
	for _, it := range items {
		if it.Status == "" {
			out.TanpaStatus++
		} else {
			l, ok := labels[it.BoardID][it.Status]
			if !ok {
				l = label{judul: it.Status, warna: "abu"}
			}
			key := strings.ToLower(l.judul)
			i, ok := slot[key]
			if !ok {
				i = len(out.Status)
				slot[key] = i
				out.Status = append(out.Status, StatusSlice{Label: l.judul, Warna: l.warna})
			}
			out.Status[i].Jumlah++
		}
		if it.Status == "done" {
			doneNow[it.ID] = true
			continue
		}
		open[it.ID] = openItem{span: spanOf(it.Mulai, it.Selesai)}
		if pk.hasPerson[it.BoardID] && it.JumlahPerson == 0 {
			out.BelumDitugaskan++
		}
	}

	// Beban kerja per orang.
	byUser := map[uuid.UUID]*Workload{}
	for _, a := range assignees {
		oi, ok := open[a.ItemID]
		if !ok {
			continue
		}
		w := byUser[a.UserID]
		if w == nil {
			w = &Workload{UserID: a.UserID, Nama: a.Nama}
			byUser[a.UserID] = w
		}
		w.ItemTerbuka++
		if oi.span != nil {
			w.HariKerja += cal.WorkdaysIn(oi.span.Start, oi.span.End+1)
			if oi.span.End < today {
				w.Terlambat++
			}
		}
	}
	for _, w := range byUser {
		out.Beban = append(out.Beban, *w)
	}
	slices.SortFunc(out.Beban, func(a, b Workload) int {
		if a.ItemTerbuka != b.ItemTerbuka {
			return b.ItemTerbuka - a.ItemTerbuka
		}
		return strings.Compare(a.Nama, b.Nama)
	})

	// Tren: transisi terakhir ke Done untuk item yang saat ini Done.
	for _, t := range transitions {
		if !doneNow[t.ItemID] {
			continue
		}
		w := (mondayOf(schedule.DayOf(t.Waktu.In(now.Location()))) - firstWeek) / 7
		if w >= 0 && w < TrendWeeks {
			out.Tren[w].Selesai++
		}
	}

	// Keuangan per proyek.
	byProject := map[uuid.UUID]dbgen.BudgetTotalsRow{}
	for _, t := range totals {
		byProject[t.ProjectID] = t
	}
	for _, r := range rows {
		f := Finance{ProjectID: r.ID, Nama: r.Nama, Anggaran: "0.00", Realisasi: "0.00", Sisa: "0.00"}
		if t, ok := byProject[r.ID]; ok {
			f.Anggaran, f.Realisasi, f.Sisa, f.JumlahEntri = t.Anggaran, t.Realisasi, t.Sisa, t.JumlahEntri
		}
		out.Keuangan = append(out.Keuangan, f)
	}
	return out, nil
}

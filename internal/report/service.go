package report

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"jwm/internal/auth"
	"jwm/internal/db/gen"
	"jwm/internal/project"
	"jwm/internal/schedule"
)

// ErrInvalidFilter dipakai untuk parameter filter yang tidak valid.
var ErrInvalidFilter = errors.New("filter tidak valid")

// Service menghitung dasbor portofolio.
type Service struct {
	pool *pgxpool.Pool
	q    *dbgen.Queries
	th   Thresholds
	// now dapat diganti tes. ASSUMPTION: "hari ini" memakai tanggal lokal server (WIB).
	now func() time.Time
}

// NewService membuat layanan dasbor.
func NewService(pool *pgxpool.Pool, th Thresholds) *Service {
	return &Service{pool: pool, q: dbgen.New(pool), th: th, now: time.Now}
}

// SetClock mengganti sumber waktu (untuk tes).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Filter menyaring dasbor. Field kosong berarti tidak disaring.
type Filter struct {
	Prioritas []string
	PemilikID *uuid.UUID
	Status    []string
	// PortfolioID; TanpaPortofolio memilih proyek yang belum masuk portofolio.
	PortfolioID     *uuid.UUID
	TanpaPortofolio bool
}

// ProjectHealth adalah satu baris dasbor portofolio.
type ProjectHealth struct {
	ID            uuid.UUID  `json:"id"`
	Nama          string     `json:"nama"`
	PemilikID     uuid.UUID  `json:"pemilik_id"`
	PemilikNama   string     `json:"pemilik_nama"`
	Prioritas     string     `json:"prioritas"`
	Mulai         *string    `json:"mulai"`
	Selesai       *string    `json:"selesai"`
	PortfolioID   *uuid.UUID `json:"portfolio_id"`
	PortfolioNama *string    `json:"portfolio_nama"`

	Progres         *float64       `json:"progres"`
	ProgresRencana  *float64       `json:"progres_rencana"`
	ProgresJadwal   *float64       `json:"progres_jadwal"`
	JumlahItem      int            `json:"jumlah_item"`
	ItemBerisiko    int            `json:"item_berisiko"`
	PerAturan       map[string]int `json:"per_aturan"`
	KritisTerlambat int            `json:"kritis_terlambat"`

	// Health adalah status efektif: status manual bila ada, selain itu otomatis.
	Health         string                `json:"health"`
	HealthOtomatis string                `json:"health_otomatis"`
	HealthAlasan   []string              `json:"health_alasan"`
	StatusManual   *project.ManualStatus `json:"status_manual"`
	Peringatan     []string              `json:"peringatan"`
}

// Dashboard adalah respons GET /portfolio.
type Dashboard struct {
	Items   []ProjectHealth `json:"items"`
	Ambang  Thresholds      `json:"ambang"`
	HariIni string          `json:"hari_ini"`
}

func dateStr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := schedule.FormatDay(schedule.DayOf(d.Time))
	return &s
}

// visibleProjects mengembalikan proyek aktif yang boleh dilihat pelaku, sama
// dengan daftar proyek: Admin dan PMO semua, selain itu proyek keanggotaan.
func (s *Service) visibleProjects(ctx context.Context, actor auth.Principal) ([]dbgen.ListDashboardProjectsRow, error) {
	g := actor.User.Peran
	all := g == auth.RoleAdmin || g == auth.RolePMO
	rows, err := s.q.ListDashboardProjects(ctx, dbgen.ListDashboardProjectsParams{UserID: actor.User.ID, Semua: all})
	if err != nil {
		return nil, fmt.Errorf("daftar proyek: %w", err)
	}
	if all {
		return rows, nil
	}
	out := rows[:0]
	for _, r := range rows {
		role, _ := project.ParseRole(deref(r.PeranProyek))
		if project.Effective(g, true, role) >= project.RoleViewer {
			out = append(out, r)
		}
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (f Filter) match(r dbgen.ListDashboardProjectsRow) bool {
	if len(f.Prioritas) > 0 && !slices.Contains(f.Prioritas, r.Prioritas) {
		return false
	}
	if f.PemilikID != nil && r.PemilikID != *f.PemilikID {
		return false
	}
	if f.TanpaPortofolio && r.PortfolioID != nil {
		return false
	}
	if f.PortfolioID != nil && (r.PortfolioID == nil || *r.PortfolioID != *f.PortfolioID) {
		return false
	}
	return true
}

// Dashboard menghitung health semua proyek yang terlihat pelaku.
func (s *Service) Dashboard(ctx context.Context, actor auth.Principal, f Filter) (Dashboard, error) {
	for _, p := range f.Prioritas {
		if p != "rendah" && p != "sedang" && p != "tinggi" && p != "kritis" {
			return Dashboard{}, fmt.Errorf("%w: prioritas %q", ErrInvalidFilter, p)
		}
	}
	for _, st := range f.Status {
		if st != Green && st != Yellow && st != Red {
			return Dashboard{}, fmt.Errorf("%w: status %q", ErrInvalidFilter, st)
		}
	}
	rows, err := s.visibleProjects(ctx, actor)
	if err != nil {
		return Dashboard{}, err
	}
	rows = slices.DeleteFunc(rows, func(r dbgen.ListDashboardProjectsRow) bool { return !f.match(r) })
	today := schedule.DayOf(s.now())
	results, err := s.evaluate(ctx, rows, today)
	if err != nil {
		return Dashboard{}, err
	}
	out := Dashboard{Items: []ProjectHealth{}, Ambang: s.th, HariIni: schedule.FormatDay(today)}
	for _, r := range rows {
		ev := results[r.ID]
		ph := ProjectHealth{
			ID: r.ID, Nama: r.Nama, PemilikID: r.PemilikID, PemilikNama: r.PemilikNama, Prioritas: r.Prioritas,
			Mulai: dateStr(r.Mulai), Selesai: dateStr(r.Selesai), PortfolioID: r.PortfolioID, PortfolioNama: r.PortfolioNama,
			Progres: ev.res.Progres, ProgresRencana: ev.res.ProgresRencana, ProgresJadwal: ev.res.ProgresJadwal,
			JumlahItem: ev.res.JumlahItem, ItemBerisiko: ev.res.ItemBerisiko, PerAturan: ev.res.PerAturan,
			KritisTerlambat: ev.res.KritisTerlambat, Health: ev.res.Health, HealthOtomatis: ev.res.Health,
			HealthAlasan: ev.res.Alasan, Peringatan: ev.warnings,
		}
		if r.StatusManual != nil {
			ph.Health = *r.StatusManual
			ph.StatusManual = &project.ManualStatus{
				Status: *r.StatusManual, Alasan: deref(r.StatusManualAlasan), OlehNama: deref(r.StatusManualOlehNama),
			}
			if r.StatusManualPada != nil {
				ph.StatusManual.Pada = *r.StatusManualPada
			}
		}
		if len(f.Status) > 0 && !slices.Contains(f.Status, ph.Health) {
			continue
		}
		out.Items = append(out.Items, ph)
	}
	return out, nil
}

// RiskItem adalah item berisiko pada rincian satu proyek.
type RiskItem struct {
	ItemRisk
	BoardNama string `json:"board_nama"`
}

// ProjectRisks adalah respons GET /portfolio/projects/{id}/risks.
type ProjectRisks struct {
	ProjectID uuid.UUID  `json:"project_id"`
	Items     []RiskItem `json:"items"`
}

// Risks mengembalikan item berisiko satu proyek beserta alasannya. Hak baca proyek.
func (s *Service) Risks(ctx context.Context, actor auth.Principal, projectID uuid.UUID) (ProjectRisks, error) {
	if _, err := project.Access(ctx, s.q, actor, projectID, project.RoleViewer, false); err != nil {
		return ProjectRisks{}, err
	}
	rows, err := s.visibleProjects(ctx, actor)
	if err != nil {
		return ProjectRisks{}, err
	}
	rows = slices.DeleteFunc(rows, func(r dbgen.ListDashboardProjectsRow) bool { return r.ID != projectID })
	if len(rows) == 0 {
		// Proyek arsip tidak dievaluasi.
		return ProjectRisks{ProjectID: projectID, Items: []RiskItem{}}, nil
	}
	results, err := s.evaluate(ctx, rows, schedule.DayOf(s.now()))
	if err != nil {
		return ProjectRisks{}, err
	}
	ev := results[projectID]
	out := ProjectRisks{ProjectID: projectID, Items: make([]RiskItem, 0, len(ev.res.Items))}
	for _, it := range ev.res.Items {
		out.Items = append(out.Items, RiskItem{ItemRisk: it, BoardNama: ev.boards[it.BoardID]})
	}
	return out, nil
}

type evaluation struct {
	res      ProjectResult
	warnings []string
	boards   map[uuid.UUID]string
}

// evaluate memuat board, kolom, nilai item, dan dependensi semua proyek dalam
// beberapa query, lalu menghitung CPM per board dan risiko per proyek.
func (s *Service) evaluate(ctx context.Context, rows []dbgen.ListDashboardProjectsRow, today int) (map[uuid.UUID]evaluation, error) {
	out := make(map[uuid.UUID]evaluation, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	projectIDs := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		projectIDs[i] = r.ID
	}
	q, done, err := s.readTx(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	boards, err := q.ListBoardsOfProjects(ctx, projectIDs)
	if err != nil {
		return nil, fmt.Errorf("daftar board: %w", err)
	}
	boardIDs := make([]uuid.UUID, len(boards))
	boardNames := map[uuid.UUID]map[uuid.UUID]string{}
	for i, b := range boards {
		boardIDs[i] = b.ID
		if boardNames[b.ProjectID] == nil {
			boardNames[b.ProjectID] = map[uuid.UUID]string{}
		}
		boardNames[b.ProjectID][b.ID] = b.Nama
	}
	// Kolom pertama tiap tipe per board (urutan tampilan).
	cols, err := q.ListColumnsOfBoards(ctx, boardIDs)
	if err != nil {
		return nil, fmt.Errorf("daftar kolom: %w", err)
	}
	pk := pickColumns(cols)
	hasPerson := pk.hasPerson
	p := pk.params
	p.BoardIds = boardIDs
	rows2, err := q.ListPortfolioItems(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("baca item: %w", err)
	}
	deps, err := q.ListDependenciesOfBoards(ctx, boardIDs)
	if err != nil {
		return nil, fmt.Errorf("daftar dependensi: %w", err)
	}
	cal, err := schedule.LoadCalendar(ctx, q)
	if err != nil {
		return nil, err
	}

	// Fakta per item dari nilai skalar query (lihat ListPortfolioItems).
	facts := make([]ItemFacts, len(rows2))
	itemsByBoard := map[uuid.UUID][]*ItemFacts{}
	for i, v := range rows2 {
		f := &facts[i]
		*f = ItemFacts{
			ID: v.ID, BoardID: v.BoardID, Judul: v.Judul, Done: v.Status == "done",
			HasAssigneeCol: hasPerson[v.BoardID], Assigned: v.JumlahPerson > 0,
		}
		switch {
		case v.Progres >= 0:
			f.Progress = min(v.Progres, 100)
		case f.Done:
			f.Progress = 100
		}
		f.Span = spanOf(v.Mulai, v.Selesai)
		itemsByBoard[f.BoardID] = append(itemsByBoard[f.BoardID], f)
	}
	edgesByBoard := map[uuid.UUID][]schedule.Edge{}
	predsOf := map[uuid.UUID][]uuid.UUID{}
	for _, d := range deps {
		edgesByBoard[d.BoardID] = append(edgesByBoard[d.BoardID],
			schedule.Edge{ID: d.ID, Pred: d.PredecessorID, Succ: d.SuccessorID, Lag: int(d.LagHari)})
		predsOf[d.SuccessorID] = append(predsOf[d.SuccessorID], d.PredecessorID)
	}

	// CPM per board pada timeline utama.
	projectItems := map[uuid.UUID][]ItemFacts{}
	warnings := map[uuid.UUID][]string{}
	for _, b := range boards {
		items := itemsByBoard[b.ID]
		nodes := make([]schedule.Node, len(items))
		for i, it := range items {
			nodes[i] = schedule.Node{ID: it.ID, Judul: it.Judul, Span: it.Span}
		}
		sched := map[uuid.UUID]*schedule.ItemSchedule{}
		if len(nodes) > 0 {
			res, err := schedule.Compute(nodes, edgesByBoard[b.ID], cal)
			if err != nil {
				warnings[b.ProjectID] = append(warnings[b.ProjectID],
					fmt.Sprintf("Jalur kritis board %q tidak dapat dihitung: %v", b.Nama, err))
			}
			for i := range res.Items {
				sched[res.Items[i].ItemID] = &res.Items[i]
			}
		}
		for _, it := range items {
			it.Sched = sched[it.ID]
			it.Preds = predsOf[it.ID]
			projectItems[b.ProjectID] = append(projectItems[b.ProjectID], *it)
		}
	}

	for _, r := range rows {
		in := ProjectInput{Items: projectItems[r.ID], Today: today, Cal: cal}
		if r.Selesai.Valid {
			d := schedule.DayOf(r.Selesai.Time)
			in.Deadline = &d
		}
		w := warnings[r.ID]
		if w == nil {
			w = []string{}
		}
		out[r.ID] = evaluation{res: Evaluate(in, s.th), warnings: w, boards: boardNames[r.ID]}
	}
	return out, nil
}

// spanOf membaca tanggal timeline; nil bila kosong, tidak valid, atau terbalik.
func spanOf(mulai, selesai string) *schedule.Span {
	s, err1 := schedule.ParseDay(mulai)
	e, err2 := schedule.ParseDay(selesai)
	if err1 != nil || err2 != nil || e < s {
		return nil
	}
	return &schedule.Span{Start: s, End: e}
}

// readTx membuka transaksi read-only (satu snapshot untuk semua query dasbor)
// dengan work_mem cukup agar agregasi tidak tumpah ke disk.
func (s *Service) readTx(ctx context.Context) (*dbgen.Queries, func(), error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, nil, fmt.Errorf("mulai transaksi: %w", err)
	}
	done := func() { _ = tx.Rollback(ctx) }
	q := s.q.WithTx(tx)
	if err := q.SetDashboardWorkMem(ctx); err != nil {
		done()
		return nil, nil, fmt.Errorf("atur work_mem: %w", err)
	}
	return q, done, nil
}

// picks adalah kolom pertama tiap tipe per board (urutan tampilan).
type picks struct {
	params    dbgen.ListPortfolioItemsParams
	hasPerson map[uuid.UUID]bool
	// status: kolom status terpilih beserta konfigurasi labelnya.
	status map[uuid.UUID]dbgen.ListColumnsOfBoardsRow
}

func pickColumns(cols []dbgen.ListColumnsOfBoardsRow) picks {
	out := picks{hasPerson: map[uuid.UUID]bool{}, status: map[uuid.UUID]dbgen.ListColumnsOfBoardsRow{}}
	seen := map[[2]string]bool{}
	for _, c := range cols {
		key := [2]string{c.BoardID.String(), c.Tipe}
		if seen[key] {
			continue
		}
		seen[key] = true
		switch c.Tipe {
		case "status":
			out.params.StatusCols = append(out.params.StatusCols, c.ID)
			out.status[c.ID] = c
		case "progress":
			out.params.ProgressCols = append(out.params.ProgressCols, c.ID)
		case "person":
			out.params.PersonCols = append(out.params.PersonCols, c.ID)
			out.hasPerson[c.BoardID] = true
		case "timeline":
			out.params.TimelineCols = append(out.params.TimelineCols, c.ID)
		}
	}
	return out
}

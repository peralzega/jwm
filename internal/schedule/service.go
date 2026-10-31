package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"jwm/internal/audit"
	"jwm/internal/auth"
	"jwm/internal/board"
	"jwm/internal/db/gen"
	"jwm/internal/project"
)

// Galat jadwal.
var (
	ErrNoTimeline      = errors.New("board ini belum memiliki kolom timeline")
	ErrNotTimeline     = errors.New("kolom bukan kolom timeline pada board ini")
	ErrScheduleTooWide = errors.New("jadwal melewati rentang kalender yang didukung; periksa tanggal dan lag")
)

// Warning adalah peringatan pada hasil jadwal.
type Warning struct {
	Kode  string `json:"kode"`
	Tahun int    `json:"tahun,omitempty"`
	Pesan string `json:"pesan"`
}

// ScheduleView adalah respons GET /boards/{id}/schedule.
type ScheduleView struct {
	KolomID uuid.UUID `json:"kolom_id"`
	// KolomUtama: kolom timeline pertama board. Pergeseran otomatis hanya
	// berlaku pada kolom ini.
	KolomUtama          bool           `json:"kolom_utama"`
	SelesaiProyek       *string        `json:"selesai_proyek"`
	Items               []ItemSchedule `json:"items"`
	DependensiDilanggar []uuid.UUID    `json:"dependensi_dilanggar"`
	Peringatan          []Warning      `json:"peringatan"`
}

// timelineColumns mengembalikan kolom timeline board urut tampilan; yang
// pertama adalah kolom utama.
func timelineColumns(ctx context.Context, q *dbgen.Queries, boardID uuid.UUID) ([]uuid.UUID, error) {
	cols, err := q.ListColumns(ctx, boardID)
	if err != nil {
		return nil, fmt.Errorf("daftar kolom: %w", err)
	}
	var out []uuid.UUID
	for _, c := range cols {
		if board.ColumnType(c.Tipe) == board.TypeTimeline {
			out = append(out, c.ID)
		}
	}
	return out, nil
}

// parseTimeline membaca nilai {"mulai","selesai"}; nilai kosong atau tidak
// valid dianggap tanpa jadwal.
func parseTimeline(raw []byte) *Span {
	if len(raw) == 0 {
		return nil
	}
	var tl board.Timeline
	if json.Unmarshal(raw, &tl) != nil {
		return nil
	}
	s, err1 := parseDay(tl.Mulai)
	e, err2 := parseDay(tl.Selesai)
	if err1 != nil || err2 != nil || e < s {
		return nil
	}
	return &Span{Start: s, End: e}
}

type boardGraph struct {
	nodes []Node
	edges []Edge
	raw   map[uuid.UUID][]byte // nilai timeline asli per item
	cal   *Calendar
}

func loadCalendar(ctx context.Context, q *dbgen.Queries) (*Calendar, error) {
	rows, err := q.ListWorkCalendar(ctx)
	if err != nil {
		return nil, fmt.Errorf("baca kalender kerja: %w", err)
	}
	days := make([]CalendarDay, len(rows))
	for i, r := range rows {
		days[i] = CalendarDay{Tanggal: r.Tanggal, HariKerja: r.HariKerja, Keterangan: r.Keterangan}
	}
	return NewCalendar(days)
}

func loadBoardGraph(ctx context.Context, q *dbgen.Queries, boardID, columnID uuid.UUID) (*boardGraph, error) {
	rows, err := q.ListTimelineValues(ctx, dbgen.ListTimelineValuesParams{ColumnID: columnID, BoardID: boardID})
	if err != nil {
		return nil, fmt.Errorf("baca timeline: %w", err)
	}
	deps, err := q.ListDependencies(ctx, boardID)
	if err != nil {
		return nil, fmt.Errorf("daftar dependensi: %w", err)
	}
	cal, err := loadCalendar(ctx, q)
	if err != nil {
		return nil, err
	}
	bg := &boardGraph{cal: cal, raw: make(map[uuid.UUID][]byte, len(rows))}
	bg.nodes = make([]Node, len(rows))
	for i, r := range rows {
		bg.nodes[i] = Node{ID: r.ID, Judul: r.Judul, Span: parseTimeline(r.NilaiJson)}
		bg.raw[r.ID] = r.NilaiJson
	}
	bg.edges = make([]Edge, len(deps))
	for i, d := range deps {
		bg.edges[i] = Edge{ID: d.ID, Pred: d.PredecessorID, Succ: d.SuccessorID, Lag: int(d.LagHari)}
	}
	return bg, nil
}

func mapRangeErr(err error) error {
	if errors.Is(err, errIndexRange) {
		return fmt.Errorf("%w: %w", ErrScheduleTooWide, err)
	}
	return err
}

// Schedule menghitung CPM board pada kolom timeline columnID (nil = kolom utama).
func (s *Service) Schedule(ctx context.Context, actor auth.Principal, boardID uuid.UUID, columnID *uuid.UUID) (ScheduleView, error) {
	if err := board.BoardForRead(ctx, s.q, actor, boardID); err != nil {
		return ScheduleView{}, err
	}
	cols, err := timelineColumns(ctx, s.q, boardID)
	if err != nil {
		return ScheduleView{}, err
	}
	if len(cols) == 0 {
		return ScheduleView{}, ErrNoTimeline
	}
	col := cols[0]
	if columnID != nil {
		if !slices.Contains(cols, *columnID) {
			return ScheduleView{}, ErrNotTimeline
		}
		col = *columnID
	}
	bg, err := loadBoardGraph(ctx, s.q, boardID, col)
	if err != nil {
		return ScheduleView{}, err
	}
	res, err := Compute(bg.nodes, bg.edges, bg.cal)
	if err != nil {
		return ScheduleView{}, mapRangeErr(err)
	}
	view := ScheduleView{
		KolomID: col, KolomUtama: col == cols[0], SelesaiProyek: res.SelesaiProyek,
		Items: res.Items, DependensiDilanggar: res.DependensiDilanggar, Peringatan: []Warning{},
	}
	if res.Cakupan != nil {
		for y := yearOf(res.Cakupan.Start); y <= yearOf(res.Cakupan.End); y++ {
			if !bg.cal.HasYear(y) {
				view.Peringatan = append(view.Peringatan, Warning{
					Kode: "kalender_kosong", Tahun: y,
					Pesan: fmt.Sprintf("Kalender kerja %d belum diisi. Hanya Sabtu dan Minggu yang dihitung libur.", y),
				})
			}
		}
	}
	return view, nil
}

// LockBoard memenuhi board.Cascader: kunci yang sama dengan pembuatan dependensi.
func (s *Service) LockBoard(ctx context.Context, q *dbgen.Queries, boardID uuid.UUID) error {
	if err := q.LockBoardDependencies(ctx, boardID.String()); err != nil {
		return fmt.Errorf("kunci dependensi board: %w", err)
	}
	return nil
}

// AfterTimelineChange memenuhi board.Cascader. Bila item pada kolom timeline
// utama berubah, successor yang melanggar FS digeser maju. Pelaku harus berhak
// mengubah setiap successor yang digeser; bila tidak, seluruh perubahan ditolak.
// Audit mencatat pelaku asli dan item penyebab.
func (s *Service) AfterTimelineChange(ctx context.Context, q *dbgen.Queries, actor auth.Principal, meta auth.RequestMeta,
	item dbgen.Item, columnID uuid.UUID) ([]board.ShiftedItem, error) {
	cols, err := timelineColumns(ctx, q, item.BoardID)
	if err != nil {
		return nil, err
	}
	if len(cols) == 0 || cols[0] != columnID {
		return nil, nil
	}
	bg, err := loadBoardGraph(ctx, q, item.BoardID, columnID)
	if err != nil {
		return nil, err
	}
	shifts, err := Cascade(bg.nodes, bg.edges, bg.cal, item.ID)
	if err != nil {
		return nil, mapRangeErr(err)
	}
	if len(shifts) == 0 {
		return nil, nil
	}

	// Kunci dan periksa hak dalam urutan id untuk mencegah deadlock.
	ids := make([]uuid.UUID, len(shifts))
	for i, sh := range shifts {
		ids[i] = sh.ItemID
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	var denied []string
	for _, id := range ids {
		_, err := board.ItemForEdit(ctx, q, actor, id)
		if errors.Is(err, project.ErrForbidden) {
			denied = append(denied, id.String())
			continue
		}
		if err != nil {
			return nil, err
		}
	}
	if len(denied) > 0 {
		names := make([]string, 0, len(denied))
		for _, sh := range shifts {
			if slices.Contains(denied, sh.ItemID.String()) {
				names = append(names, strconv.Quote(sh.Judul))
			}
		}
		return nil, fmt.Errorf("%w: %s", board.ErrShiftForbidden, strings.Join(names, ", "))
	}

	out := make([]board.ShiftedItem, len(shifts))
	for i, sh := range shifts {
		after, err := json.Marshal(board.Timeline{Mulai: sh.Mulai, Selesai: sh.Selesai})
		if err != nil {
			return nil, fmt.Errorf("susun timeline: %w", err)
		}
		if err := q.UpsertItemValue(ctx, dbgen.UpsertItemValueParams{
			ItemID: sh.ItemID, ColumnID: columnID, NilaiJson: after, DiubahOleh: &actor.User.ID,
		}); err != nil {
			return nil, fmt.Errorf("simpan pergeseran: %w", err)
		}
		if err := q.TouchItem(ctx, sh.ItemID); err != nil {
			return nil, fmt.Errorf("perbarui item: %w", err)
		}
		if err := audit.Record(ctx, q, audit.Entry{
			AktorID: &actor.User.ID, Entitas: "item", EntitasID: sh.ItemID.String(), Aksi: "geser_otomatis",
			Sebelum: map[string]any{"column_id": columnID, "tipe": board.TypeTimeline, "nilai": json.RawMessage(bg.raw[sh.ItemID])},
			Sesudah: map[string]any{
				"column_id": columnID, "tipe": board.TypeTimeline, "nilai": json.RawMessage(after),
				"penyebab_id": item.ID, "penyebab_judul": item.Judul, "predecessor_id": sh.PredecessorID,
			},
			IP: meta.IP,
		}); err != nil {
			return nil, err
		}
		out[i] = board.ShiftedItem{
			ID: sh.ItemID, Judul: sh.Judul, Mulai: sh.Mulai, Selesai: sh.Selesai, PredecessorID: sh.PredecessorID,
		}
	}
	return out, nil
}

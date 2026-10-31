package schedule

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
)

// Perhitungan jalur kritis (CPM) dan pergeseran otomatis successor (SPEC M4).
//
// Aturan yang dipakai (didokumentasikan di docs/SPEC.md M4):
//   - Waktu diukur dalam titik hari kerja (lihat workIndex). Durasi item adalah
//     banyaknya hari kerja di [mulai, selesai]; lag juga dalam hari kerja.
//   - Tanggal mulai terjadwal adalah batasan "mulai tidak lebih awal dari".
//     ES = maks(mulai terjadwal, EF predecessor + lag). Tanggal selesai proyek
//     adalah EF terbesar, dan LF diturunkan mundur darinya.
//   - Item tanpa jadwal di tengah rantai diperlakukan sebagai titik lewat
//     berdurasi nol: rantai tidak terputus, dan batasannya diteruskan ke
//     successor. Item tanpa jadwal yang tidak punya predecessor berjadwal tidak
//     memiliki ES (tidak dihitung). Keduanya ditandai tanpa_jadwal.
//   - Item induk dan subitem (induk_id) diperlakukan sebagai item biasa; tidak
//     ada penjumlahan jadwal subitem ke induk pada v1.
//   - Hanya dependensi FS.

// Span adalah jadwal inklusif [Start, End] dalam nomor hari.
type Span struct {
	Start int
	End   int
}

// Node adalah satu item dalam graf jadwal. Span nil berarti tanpa jadwal.
type Node struct {
	ID    uuid.UUID
	Judul string
	Span  *Span
}

// Edge adalah dependensi FS dengan lag dalam hari kerja.
type Edge struct {
	ID   uuid.UUID
	Pred uuid.UUID
	Succ uuid.UUID
	Lag  int
}

type link struct {
	edge  int // indeks di edges
	other int // indeks node di ujung lain
	lag   int
}

type cpmNode struct {
	Node
	scheduled bool
	s, f, dur int // titik mulai, titik selesai (eksklusif), durasi
	preds     []link
	succs     []link

	defined        bool // ES dapat dihitung
	es, ef, ls, lf int
	plannedOK      bool // titik selesai terjadwal (atau turunan untuk titik lewat) diketahui
	planned        int
	violates       bool
}

type graph struct {
	idx   *workIndex
	nodes []*cpmNode
	byID  map[uuid.UUID]int
	edges []Edge
	order []int // urutan topologis
}

// buildGraph menyusun graf: semua item berjadwal, ditambah item tanpa jadwal
// yang terhubung dependensi. Dependensi ke item yang tidak dikenal diabaikan.
func buildGraph(nodes []Node, edges []Edge, idx *workIndex) (*graph, error) {
	g := &graph{idx: idx, byID: map[uuid.UUID]int{}}
	linked := map[uuid.UUID]bool{}
	known := map[uuid.UUID]bool{}
	for _, n := range nodes {
		known[n.ID] = true
	}
	for _, e := range edges {
		if known[e.Pred] && known[e.Succ] {
			linked[e.Pred], linked[e.Succ] = true, true
		}
	}
	for _, n := range nodes {
		if n.Span == nil && !linked[n.ID] {
			continue
		}
		cn := &cpmNode{Node: n}
		if n.Span != nil {
			s, err := idx.point(n.Span.Start)
			if err != nil {
				return nil, err
			}
			f, err := idx.point(n.Span.End + 1)
			if err != nil {
				return nil, err
			}
			cn.scheduled, cn.s, cn.f, cn.dur = true, s, f, f-s
		}
		g.byID[n.ID] = len(g.nodes)
		g.nodes = append(g.nodes, cn)
	}
	for _, e := range edges {
		p, okP := g.byID[e.Pred]
		s, okS := g.byID[e.Succ]
		if !okP || !okS {
			continue
		}
		i := len(g.edges)
		g.edges = append(g.edges, e)
		g.nodes[p].succs = append(g.nodes[p].succs, link{edge: i, other: s, lag: e.Lag})
		g.nodes[s].preds = append(g.nodes[s].preds, link{edge: i, other: p, lag: e.Lag})
	}
	return g, g.topoSort()
}

// topoSort mengisi g.order (Kahn). Siklus seharusnya sudah ditolak saat
// dependensi dibuat; bila tetap ada (data lama), perhitungan dihentikan.
func (g *graph) topoSort() error {
	indeg := make([]int, len(g.nodes))
	for i, n := range g.nodes {
		indeg[i] = len(n.preds)
	}
	queue := make([]int, 0, len(g.nodes))
	for i, d := range indeg {
		if d == 0 {
			queue = append(queue, i)
		}
	}
	for h := 0; h < len(queue); h++ {
		for _, l := range g.nodes[queue[h]].succs {
			indeg[l.other]--
			if indeg[l.other] == 0 {
				queue = append(queue, l.other)
			}
		}
	}
	if len(queue) != len(g.nodes) {
		return fmt.Errorf("%w: data dependensi board berisi siklus", ErrCycle)
	}
	g.order = queue
	return nil
}

// cpmResult adalah hasil CPM dalam titik hari kerja.
type cpmResult struct {
	g        *graph
	finish   int // titik selesai proyek (eksklusif)
	any      bool
	violated []int // indeks edge yang dilanggar jadwal terjadwal
}

func computeCPM(g *graph) *cpmResult {
	r := &cpmResult{g: g}
	for _, i := range g.order {
		n := g.nodes[i]
		if n.scheduled {
			n.defined, n.es = true, n.s
			n.plannedOK, n.planned = true, n.f
		}
		for _, l := range n.preds {
			p := g.nodes[l.other]
			if p.defined && (!n.defined || p.ef+l.lag > n.es) {
				n.defined, n.es = true, p.ef+l.lag
			}
			if !p.plannedOK {
				continue
			}
			req := p.planned + l.lag
			switch {
			case n.scheduled:
				if n.s < req {
					n.violates = true
					r.violated = append(r.violated, l.edge)
				}
			case !n.plannedOK || req > n.planned:
				n.plannedOK, n.planned = true, req
			}
		}
		if n.defined {
			n.ef = n.es + n.dur
			if !r.any || n.ef > r.finish {
				r.any, r.finish = true, n.ef
			}
		}
	}
	for k := len(g.order) - 1; k >= 0; k-- {
		n := g.nodes[g.order[k]]
		if !n.defined {
			continue
		}
		n.lf = r.finish
		for _, l := range n.succs {
			if s := g.nodes[l.other]; s.defined && s.ls-l.lag < n.lf {
				n.lf = s.ls - l.lag
			}
		}
		n.ls = n.lf - n.dur
	}
	sort.Ints(r.violated)
	return r
}

// ItemSchedule adalah hasil CPM satu item, dalam tanggal YYYY-MM-DD.
// ES sampai LF bernilai null untuk item tanpa jadwal yang tidak dapat dihitung.
type ItemSchedule struct {
	ItemID      uuid.UUID `json:"item_id"`
	ES          *string   `json:"es"`
	EF          *string   `json:"ef"`
	LS          *string   `json:"ls"`
	LF          *string   `json:"lf"`
	Slack       *int      `json:"slack"`
	Kritis      bool      `json:"kritis"`
	MelanggarFS bool      `json:"melanggar_fs"`
	TanpaJadwal bool      `json:"tanpa_jadwal"`
}

// dates mengubah titik mulai dan durasi menjadi tanggal. Item berdurasi nol
// (titik lewat, atau seluruhnya di hari libur) memakai tanggal yang sama.
func (r *cpmResult) dates(start, dur int) (string, string, error) {
	s, err := r.g.idx.dayAt(start)
	if err != nil {
		return "", "", err
	}
	e := s
	if dur > 0 {
		if e, err = r.g.idx.dayAt(start + dur - 1); err != nil {
			return "", "", err
		}
	}
	return formatDay(s), formatDay(e), nil
}

// items mengubah hasil menjadi daftar per item (urut id) beserta rentang hari
// yang tercakup (untuk peringatan kalender).
func (r *cpmResult) items() ([]ItemSchedule, Span, error) {
	out := make([]ItemSchedule, 0, len(r.g.nodes))
	cover := Span{}
	first := true
	extend := func(d int) {
		if first || d < cover.Start {
			cover.Start = d
		}
		if first || d > cover.End {
			cover.End = d
		}
		first = false
	}
	for _, n := range r.g.nodes {
		it := ItemSchedule{ItemID: n.ID, MelanggarFS: n.violates, TanpaJadwal: !n.scheduled}
		if n.Span != nil {
			extend(n.Span.Start)
			extend(n.Span.End)
		}
		if n.defined {
			es, ef, err := r.dates(n.es, n.dur)
			if err != nil {
				return nil, cover, err
			}
			ls, lf, err := r.dates(n.ls, n.dur)
			if err != nil {
				return nil, cover, err
			}
			slack := n.ls - n.es
			it.ES, it.EF, it.LS, it.LF, it.Slack = &es, &ef, &ls, &lf, &slack
			it.Kritis = slack <= 0
			for _, d := range []string{es, lf} {
				day, _ := parseDay(d)
				extend(day)
			}
		}
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ItemID.String() < out[j].ItemID.String() })
	return out, cover, nil
}

// Result adalah hasil CPM satu board.
type Result struct {
	Items []ItemSchedule
	// SelesaiProyek adalah EF terbesar; nil bila tidak ada item berjadwal.
	SelesaiProyek *string
	// DependensiDilanggar berisi id dependensi yang successor-nya dijadwalkan
	// mulai sebelum predecessor selesai (ditambah lag).
	DependensiDilanggar []uuid.UUID
	// Cakupan adalah rentang hari yang dipakai jadwal (untuk peringatan kalender).
	Cakupan *Span
}

func newGraph(nodes []Node, edges []Edge, cal *Calendar) (*graph, error) {
	anchor, found := 0, false
	for _, n := range nodes {
		if n.Span != nil && (!found || n.Span.Start < anchor) {
			anchor, found = n.Span.Start, true
		}
	}
	return buildGraph(nodes, edges, newWorkIndex(cal, anchor))
}

// Compute menjalankan CPM untuk satu board.
func Compute(nodes []Node, edges []Edge, cal *Calendar) (Result, error) {
	g, err := newGraph(nodes, edges, cal)
	if err != nil {
		return Result{}, err
	}
	r := computeCPM(g)
	items, cover, err := r.items()
	if err != nil {
		return Result{}, err
	}
	out := Result{Items: items, DependensiDilanggar: make([]uuid.UUID, 0, len(r.violated))}
	for _, e := range r.violated {
		out.DependensiDilanggar = append(out.DependensiDilanggar, g.edges[e].ID)
	}
	for _, it := range items {
		// YYYY-MM-DD dapat dibandingkan sebagai string.
		if it.EF != nil && (out.SelesaiProyek == nil || *it.EF > *out.SelesaiProyek) {
			out.SelesaiProyek = it.EF
		}
	}
	if out.SelesaiProyek != nil {
		out.Cakupan = &cover
	}
	return out, nil
}

// Cascade menghitung pergeseran successor setelah jadwal item changed
// berubah. nodes sudah berisi jadwal baru item tersebut.
func Cascade(nodes []Node, edges []Edge, cal *Calendar, changed uuid.UUID) ([]Shift, error) {
	g, err := newGraph(nodes, edges, cal)
	if err != nil {
		return nil, err
	}
	return cascade(g, changed)
}

// Shift adalah satu item yang digeser otomatis.
type Shift struct {
	ItemID uuid.UUID `json:"id"`
	Judul  string    `json:"judul"`
	// PredecessorID adalah predecessor langsung yang memaksa pergeseran.
	PredecessorID uuid.UUID `json:"predecessor_id"`
	Dari          Span      `json:"-"`
	Ke            Span      `json:"-"`
	Mulai         string    `json:"mulai"`
	Selesai       string    `json:"selesai"`
}

// cascade menghitung pergeseran successor setelah jadwal item changed berubah
// (g sudah berisi jadwal barunya).
//
// Aturan: hanya maju. Successor bergeser bila mulainya lebih awal dari
// selesai predecessor yang berubah ditambah lag; durasi hari kerjanya tetap
// (item berdurasi nol menjadi satu hari kerja). Predecessor yang tidak berubah
// tidak memicu pergeseran, sehingga pelanggaran lama di bagian lain board
// tidak ikut "diperbaiki" diam-diam. Item tanpa jadwal meneruskan batasan
// (titik lewat) tanpa ditulis.
func cascade(g *graph, changed uuid.UUID) ([]Shift, error) {
	start, ok := g.byID[changed]
	if !ok || !g.nodes[start].scheduled {
		return nil, nil
	}
	moved := map[int]int{start: g.nodes[start].f} // node -> titik selesai baru
	var out []Shift
	for _, i := range g.order {
		if i == start {
			continue
		}
		n := g.nodes[i]
		req, driver, has := 0, -1, false
		for _, l := range n.preds {
			f, ok := moved[l.other]
			if ok && (!has || f+l.lag > req) {
				req, driver, has = f+l.lag, l.other, true
			}
		}
		if !has {
			continue
		}
		if !n.scheduled {
			moved[i] = req
			continue
		}
		if n.s >= req {
			continue
		}
		dur := max(n.dur, 1)
		s, err := g.idx.dayAt(req)
		if err != nil {
			return nil, err
		}
		e, err := g.idx.dayAt(req + dur - 1)
		if err != nil {
			return nil, err
		}
		moved[i] = req + dur
		out = append(out, Shift{
			ItemID: n.ID, Judul: n.Judul, PredecessorID: g.nodes[driver].ID,
			Dari: *n.Span, Ke: Span{Start: s, End: e}, Mulai: formatDay(s), Selesai: formatDay(e),
		})
	}
	return out, nil
}

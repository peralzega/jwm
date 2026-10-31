package schedule

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tn adalah item uji; mulai kosong berarti tanpa jadwal.
type tn struct{ id, mulai, selesai string }

// te adalah dependensi FS uji.
type te struct {
	pred, succ string
	lag        int
}

func uid(name string) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte(name)) }

func toGraphInput(t *testing.T, nodes []tn, edges []te) ([]Node, []Edge) {
	t.Helper()
	ns := make([]Node, len(nodes))
	for i, n := range nodes {
		ns[i] = Node{ID: uid(n.id), Judul: n.id}
		if n.mulai != "" {
			ns[i].Span = &Span{Start: day(t, n.mulai), End: day(t, n.selesai)}
		}
	}
	es := make([]Edge, len(edges))
	for i, e := range edges {
		es[i] = Edge{ID: uid(e.pred + ">" + e.succ), Pred: uid(e.pred), Succ: uid(e.succ), Lag: e.lag}
	}
	return ns, es
}

// want adalah hasil yang diharapkan per item: "es ef ls lf slack" atau "-"
// untuk item tanpa ES.
type want map[string]string

func TestComputeCPM(t *testing.T) {
	// November 2026: Senin 2, Jumat 6, Sabtu 7, Minggu 8, Senin 9.
	holiday := CalendarDay{Tanggal: "2026-11-04", Keterangan: "Libur"}
	saturday := CalendarDay{Tanggal: "2026-11-07", HariKerja: true, Keterangan: "Sabtu masuk"}
	newYear := CalendarDay{Tanggal: "2027-01-01", Keterangan: "Tahun baru"}

	tests := []struct {
		name     string
		cal      []CalendarDay
		nodes    []tn
		edges    []te
		want     want
		kritis   []string
		violated []string
		tanpa    []string
		selesai  string
	}{
		{
			name:    "item tunggal",
			nodes:   []tn{{"A", "2026-11-02", "2026-11-06"}},
			want:    want{"A": "2026-11-02 2026-11-06 2026-11-02 2026-11-06 0"},
			kritis:  []string{"A"},
			selesai: "2026-11-06",
		},
		{
			name:    "rantai melewati akhir pekan",
			nodes:   []tn{{"A", "2026-11-04", "2026-11-06"}, {"B", "2026-11-09", "2026-11-10"}},
			edges:   []te{{"A", "B", 0}},
			want:    want{"A": "2026-11-04 2026-11-06 2026-11-04 2026-11-06 0", "B": "2026-11-09 2026-11-10 2026-11-09 2026-11-10 0"},
			kritis:  []string{"A", "B"},
			selesai: "2026-11-10",
		},
		{
			name: "jalur ganda: jalur pendek punya slack",
			nodes: []tn{
				{"A", "2026-11-02", "2026-11-03"}, {"B", "2026-11-02", "2026-11-05"}, {"C", "2026-11-06", "2026-11-06"},
			},
			edges: []te{{"A", "C", 0}, {"B", "C", 0}},
			want: want{
				"A": "2026-11-02 2026-11-03 2026-11-04 2026-11-05 2",
				"B": "2026-11-02 2026-11-05 2026-11-02 2026-11-05 0",
				"C": "2026-11-06 2026-11-06 2026-11-06 2026-11-06 0",
			},
			kritis:  []string{"B", "C"},
			selesai: "2026-11-06",
		},
		{
			name: "jalur ganda sama panjang: keduanya kritis",
			nodes: []tn{
				{"S", "2026-11-02", "2026-11-02"}, {"A", "2026-11-03", "2026-11-04"}, {"B", "2026-11-03", "2026-11-04"},
				{"E", "2026-11-05", "2026-11-05"},
			},
			edges:   []te{{"S", "A", 0}, {"S", "B", 0}, {"A", "E", 0}, {"B", "E", 0}},
			kritis:  []string{"A", "B", "E", "S"},
			selesai: "2026-11-05",
		},
		{
			name:     "lag positif: successor dijadwalkan terlalu awal",
			nodes:    []tn{{"A", "2026-11-02", "2026-11-03"}, {"B", "2026-11-04", "2026-11-04"}},
			edges:    []te{{"A", "B", 2}},
			want:     want{"B": "2026-11-06 2026-11-06 2026-11-06 2026-11-06 0"},
			kritis:   []string{"A", "B"},
			violated: []string{"A>B"},
			selesai:  "2026-11-06",
		},
		{
			name:    "lag negatif (lead) mengizinkan tumpang tindih",
			nodes:   []tn{{"A", "2026-11-02", "2026-11-04"}, {"B", "2026-11-04", "2026-11-05"}},
			edges:   []te{{"A", "B", -1}},
			want:    want{"B": "2026-11-04 2026-11-05 2026-11-04 2026-11-05 0"},
			kritis:  []string{"A", "B"},
			selesai: "2026-11-05",
		},
		{
			name:    "lag melewati akhir pekan dihitung hari kerja",
			nodes:   []tn{{"A", "2026-11-05", "2026-11-05"}, {"B", "2026-11-10", "2026-11-10"}},
			edges:   []te{{"A", "B", 2}},
			want:    want{"A": "2026-11-05 2026-11-05 2026-11-05 2026-11-05 0"},
			kritis:  []string{"A", "B"},
			selesai: "2026-11-10",
		},
		{
			name:    "hari libur di tengah durasi tidak dihitung",
			cal:     []CalendarDay{holiday},
			nodes:   []tn{{"A", "2026-11-02", "2026-11-05"}, {"B", "2026-11-06", "2026-11-06"}},
			edges:   []te{{"A", "B", 0}},
			want:    want{"A": "2026-11-02 2026-11-05 2026-11-02 2026-11-05 0"},
			kritis:  []string{"A", "B"},
			selesai: "2026-11-06",
		},
		{
			name:    "hari libur di titik sambung membuat predecessor kritis",
			cal:     []CalendarDay{holiday},
			nodes:   []tn{{"A", "2026-11-02", "2026-11-03"}, {"B", "2026-11-05", "2026-11-05"}},
			edges:   []te{{"A", "B", 0}},
			kritis:  []string{"A", "B"},
			selesai: "2026-11-05",
		},
		{
			name:    "tanpa hari libur, celah sehari menjadi slack",
			nodes:   []tn{{"A", "2026-11-02", "2026-11-03"}, {"B", "2026-11-05", "2026-11-05"}},
			edges:   []te{{"A", "B", 0}},
			want:    want{"A": "2026-11-02 2026-11-03 2026-11-03 2026-11-04 1"},
			kritis:  []string{"B"},
			selesai: "2026-11-05",
		},
		{
			name:    "Sabtu yang dijadikan hari kerja ikut dihitung",
			cal:     []CalendarDay{saturday},
			nodes:   []tn{{"A", "2026-11-05", "2026-11-09"}, {"B", "2026-11-10", "2026-11-10"}},
			edges:   []te{{"A", "B", 0}},
			want:    want{"A": "2026-11-05 2026-11-09 2026-11-05 2026-11-09 0"},
			kritis:  []string{"A", "B"},
			selesai: "2026-11-10",
		},
		{
			name: "grafik tak terhubung: rantai pendek punya slack",
			nodes: []tn{
				{"A", "2026-11-02", "2026-11-03"}, {"B", "2026-11-04", "2026-11-04"},
				{"X", "2026-11-02", "2026-11-09"},
			},
			edges: []te{{"A", "B", 0}},
			want: want{
				"A": "2026-11-02 2026-11-03 2026-11-05 2026-11-06 3",
				"B": "2026-11-04 2026-11-04 2026-11-09 2026-11-09 3",
				"X": "2026-11-02 2026-11-09 2026-11-02 2026-11-09 0",
			},
			kritis:  []string{"X"},
			selesai: "2026-11-09",
		},
		{
			name: "item tanpa jadwal di tengah rantai meneruskan batasan (durasi nol)",
			nodes: []tn{
				{"A", "2026-11-02", "2026-11-03"}, {"U", "", ""}, {"B", "2026-11-04", "2026-11-04"},
			},
			edges:    []te{{"A", "U", 0}, {"U", "B", 1}},
			want:     want{"U": "2026-11-04 2026-11-04 2026-11-04 2026-11-04 0", "B": "2026-11-05 2026-11-05 2026-11-05 2026-11-05 0"},
			kritis:   []string{"A", "B", "U"},
			violated: []string{"U>B"},
			tanpa:    []string{"U"},
			selesai:  "2026-11-05",
		},
		{
			name:    "item tanpa jadwal tanpa predecessor berjadwal tidak dihitung",
			nodes:   []tn{{"U", "", ""}, {"B", "2026-11-04", "2026-11-04"}, {"Z", "", ""}},
			edges:   []te{{"U", "B", 0}},
			want:    want{"U": "-", "B": "2026-11-04 2026-11-04 2026-11-04 2026-11-04 0"},
			kritis:  []string{"B"},
			tanpa:   []string{"U"},
			selesai: "2026-11-04",
		},
		{
			name:    "melewati pergantian tahun dengan libur 1 Januari",
			cal:     []CalendarDay{newYear},
			nodes:   []tn{{"A", "2026-12-30", "2027-01-04"}, {"B", "2027-01-05", "2027-01-05"}},
			edges:   []te{{"A", "B", 0}},
			want:    want{"A": "2026-12-30 2027-01-04 2026-12-30 2027-01-04 0"},
			kritis:  []string{"A", "B"},
			selesai: "2027-01-05",
		},
		{
			name:    "tahun kabisat dan batas bulan",
			nodes:   []tn{{"A", "2028-02-28", "2028-02-29"}, {"B", "2028-03-01", "2028-03-01"}, {"C", "2028-02-28", "2028-02-28"}},
			edges:   []te{{"A", "B", 0}},
			want:    want{"C": "2028-02-28 2028-02-28 2028-03-01 2028-03-01 2"},
			kritis:  []string{"A", "B"},
			selesai: "2028-03-01",
		},
		{
			name:    "item seluruhnya di akhir pekan berdurasi nol",
			nodes:   []tn{{"W", "2026-11-07", "2026-11-08"}},
			want:    want{"W": "2026-11-09 2026-11-09 2026-11-09 2026-11-09 0"},
			kritis:  []string{"W"},
			selesai: "2026-11-09",
		},
		{
			name:  "board tanpa jadwal",
			nodes: []tn{{"U", "", ""}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes, edges := toGraphInput(t, tt.nodes, tt.edges)
			res, err := Compute(nodes, edges, cal(t, tt.cal...))
			require.NoError(t, err)

			byID := map[uuid.UUID]ItemSchedule{}
			for _, it := range res.Items {
				byID[it.ItemID] = it
			}
			for name, w := range tt.want {
				it, ok := byID[uid(name)]
				require.True(t, ok, name)
				got := "-"
				if it.ES != nil {
					got = fmt.Sprintf("%s %s %s %s %d", *it.ES, *it.EF, *it.LS, *it.LF, *it.Slack)
				}
				assert.Equal(t, w, got, name)
			}
			var kritis, tanpa []string
			for _, n := range tt.nodes {
				it, ok := byID[uid(n.id)]
				if !ok {
					continue
				}
				if it.Kritis {
					kritis = append(kritis, n.id)
				}
				if it.TanpaJadwal {
					tanpa = append(tanpa, n.id)
				}
			}
			assert.ElementsMatch(t, tt.kritis, kritis, "kritis")
			assert.ElementsMatch(t, tt.tanpa, tanpa, "tanpa_jadwal")
			var violated []uuid.UUID
			for _, v := range tt.violated {
				violated = append(violated, uid(v))
			}
			assert.ElementsMatch(t, violated, res.DependensiDilanggar, "dilanggar")
			for _, v := range tt.violated {
				succ := v[len(v)-1:]
				assert.True(t, byID[uid(succ)].MelanggarFS, "melanggar_fs %s", succ)
			}
			if tt.selesai == "" {
				assert.Nil(t, res.SelesaiProyek)
			} else {
				require.NotNil(t, res.SelesaiProyek)
				assert.Equal(t, tt.selesai, *res.SelesaiProyek)
			}
		})
	}
}

func TestComputeRejectsCycle(t *testing.T) {
	nodes, edges := toGraphInput(t,
		[]tn{{"A", "2026-11-02", "2026-11-02"}, {"B", "2026-11-03", "2026-11-03"}, {"C", "", ""}},
		[]te{{"A", "B", 0}, {"B", "C", 0}, {"C", "A", 0}})
	_, err := Compute(nodes, edges, cal(t))
	assert.ErrorIs(t, err, ErrCycle)
}

func TestComputeIgnoresUnknownItems(t *testing.T) {
	nodes, edges := toGraphInput(t, []tn{{"A", "2026-11-02", "2026-11-02"}}, []te{{"A", "terhapus", 0}})
	res, err := Compute(nodes, edges, cal(t))
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	assert.Empty(t, res.DependensiDilanggar)
}

func TestCascade(t *testing.T) {
	newYear := CalendarDay{Tanggal: "2027-01-01", Keterangan: "Tahun baru"}
	tests := []struct {
		name    string
		cal     []CalendarDay
		nodes   []tn // jadwal item yang berubah sudah jadwal barunya
		edges   []te
		changed string
		want    []string // "item mulai selesai pemicu"
	}{
		{
			name: "rantai bergeser berantai, durasi hari kerja tetap",
			nodes: []tn{
				{"A", "2026-11-02", "2026-11-05"}, {"B", "2026-11-05", "2026-11-06"}, {"C", "2026-11-09", "2026-11-09"},
			},
			edges:   []te{{"A", "B", 0}, {"B", "C", 0}},
			changed: "A",
			want:    []string{"B 2026-11-06 2026-11-09 A", "C 2026-11-10 2026-11-10 B"},
		},
		{
			name:    "predecessor dipercepat: successor tidak ditarik mundur",
			nodes:   []tn{{"A", "2026-11-02", "2026-11-02"}, {"B", "2026-11-06", "2026-11-06"}},
			edges:   []te{{"A", "B", 0}},
			changed: "A",
		},
		{
			name:    "successor yang masih memenuhi FS tidak bergeser",
			nodes:   []tn{{"A", "2026-11-02", "2026-11-04"}, {"B", "2026-11-05", "2026-11-05"}},
			edges:   []te{{"A", "B", 0}},
			changed: "A",
		},
		{
			name: "pelanggaran lama dari predecessor lain tidak ikut diperbaiki",
			nodes: []tn{
				{"X", "2026-11-02", "2026-11-02"}, {"Y", "2026-11-02", "2026-11-06"}, {"B", "2026-11-04", "2026-11-04"},
			},
			edges:   []te{{"X", "B", 0}, {"Y", "B", 0}},
			changed: "X",
		},
		{
			name: "lag dan pemicu terbesar dipakai",
			nodes: []tn{
				{"A", "2026-11-02", "2026-11-04"}, {"B", "2026-11-02", "2026-11-02"}, {"C", "2026-11-03", "2026-11-04"},
			},
			edges:   []te{{"A", "B", 0}, {"A", "C", 2}, {"B", "C", 0}},
			changed: "A",
			want:    []string{"B 2026-11-05 2026-11-05 A", "C 2026-11-09 2026-11-10 A"},
		},
		{
			name:    "item tanpa jadwal meneruskan pergeseran",
			nodes:   []tn{{"A", "2026-11-02", "2026-11-04"}, {"U", "", ""}, {"B", "2026-11-03", "2026-11-03"}},
			edges:   []te{{"A", "U", 0}, {"U", "B", 1}},
			changed: "A",
			want:    []string{"B 2026-11-06 2026-11-06 U"},
		},
		{
			name:    "melewati pergantian tahun dan libur",
			cal:     []CalendarDay{newYear},
			nodes:   []tn{{"A", "2026-12-28", "2026-12-31"}, {"B", "2026-12-30", "2027-01-04"}},
			edges:   []te{{"A", "B", 0}},
			changed: "A",
			want:    []string{"B 2027-01-04 2027-01-06 A"},
		},
		{
			name:    "item berdurasi nol menjadi satu hari kerja",
			nodes:   []tn{{"A", "2026-11-02", "2026-11-06"}, {"W", "2026-11-07", "2026-11-07"}},
			edges:   []te{{"A", "W", 1}},
			changed: "A",
			want:    []string{"W 2026-11-10 2026-11-10 A"},
		},
		{
			name:    "jadwal item yang berubah dihapus: tidak ada pergeseran",
			nodes:   []tn{{"A", "", ""}, {"B", "2026-11-02", "2026-11-02"}},
			edges:   []te{{"A", "B", 0}},
			changed: "A",
		},
	}
	names := map[uuid.UUID]string{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, n := range tt.nodes {
				names[uid(n.id)] = n.id
			}
			nodes, edges := toGraphInput(t, tt.nodes, tt.edges)
			shifts, err := Cascade(nodes, edges, cal(t, tt.cal...), uid(tt.changed))
			require.NoError(t, err)
			var got []string
			for _, s := range shifts {
				got = append(got, fmt.Sprintf("%s %s %s %s", s.Judul, s.Mulai, s.Selesai, names[s.PredecessorID]))
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// randomCase membuat graf acak: DAG (edge hanya dari indeks kecil ke besar
// pada permutasi acak), lag -2..4, sebagian item tanpa jadwal, dan hari libur acak.
func randomCase(t *testing.T, rng *rand.Rand) ([]Node, []Edge, *Calendar) {
	t.Helper()
	base := day(t, "2026-01-05")
	var hol []CalendarDay
	for range rng.IntN(15) {
		hol = append(hol, CalendarDay{Tanggal: formatDay(base + rng.IntN(500)), HariKerja: rng.IntN(5) == 0})
	}
	c := cal(t, hol...)
	n := 1 + rng.IntN(30)
	perm := rng.Perm(n)
	nodes := make([]Node, n)
	for i := range nodes {
		nodes[i] = Node{ID: uid(fmt.Sprintf("n%d", i)), Judul: fmt.Sprintf("n%d", i)}
		if rng.IntN(8) != 0 {
			s := base + rng.IntN(300)
			nodes[i].Span = &Span{Start: s, End: s + rng.IntN(20)}
		}
	}
	var edges []Edge
	for a := 0; a < n; a++ {
		for b := a + 1; b < n; b++ {
			if rng.IntN(6) == 0 {
				p, s := perm[a], perm[b]
				edges = append(edges, Edge{
					ID: uid(fmt.Sprintf("e%d-%d", p, s)), Pred: nodes[p].ID, Succ: nodes[s].ID, Lag: rng.IntN(7) - 2,
				})
			}
		}
	}
	return nodes, edges, c
}

// checkInvariants memeriksa sifat CPM pada hasil dalam titik hari kerja.
func checkInvariants(t *testing.T, g *graph, r *cpmResult) {
	t.Helper()
	for _, n := range g.nodes {
		if !n.defined {
			// Tanpa ES hanya untuk item tanpa jadwal tanpa predecessor yang terhitung.
			require.False(t, n.scheduled, n.Judul)
			for _, l := range n.preds {
				require.False(t, g.nodes[l.other].defined, n.Judul)
			}
			continue
		}
		require.GreaterOrEqual(t, n.ls-n.es, 0, "slack negatif %s", n.Judul)
		require.LessOrEqual(t, n.es, n.ls, "ES > LS %s", n.Judul)
		require.Equal(t, n.es+n.dur, n.ef, n.Judul)
		require.LessOrEqual(t, n.ef, r.finish, n.Judul)
		if n.scheduled {
			require.GreaterOrEqual(t, n.es, n.s, n.Judul)
		}
		for _, l := range n.succs {
			s := g.nodes[l.other]
			require.GreaterOrEqual(t, s.es, n.ef+l.lag, "FS %s>%s", n.Judul, s.Judul)
			require.LessOrEqual(t, n.lf, s.ls-l.lag, "LF %s>%s", n.Judul, s.Judul)
		}
	}
	if !r.any {
		return
	}
	// Jalur kritis tersambung: ada item kritis yang berakhir di akhir proyek;
	// setiap item kritis yang tidak mulai pada jadwalnya sendiri punya
	// predecessor kritis yang mendorongnya, dan setiap item kritis yang tidak
	// berakhir di akhir proyek punya successor kritis yang dibatasinya.
	crit := func(n *cpmNode) bool { return n.defined && n.ls == n.es }
	endFound := false
	for _, n := range g.nodes {
		if !crit(n) {
			continue
		}
		require.Equal(t, n.lf, n.ef, "kritis tetapi LF != EF: %s", n.Judul)
		if n.ef == r.finish {
			endFound = true
		}
		if !n.scheduled || n.es > n.s {
			found := false
			for _, l := range n.preds {
				p := g.nodes[l.other]
				found = found || (crit(p) && p.ef+l.lag == n.es)
			}
			require.True(t, found, "item kritis %s tanpa predecessor kritis", n.Judul)
		}
		if n.lf < r.finish {
			found := false
			for _, l := range n.succs {
				s := g.nodes[l.other]
				found = found || (crit(s) && s.ls-l.lag == n.lf)
			}
			require.True(t, found, "item kritis %s tanpa successor kritis", n.Judul)
		}
	}
	require.True(t, endFound, "tidak ada item kritis di akhir proyek")
}

func TestCPMPropertiesRandomGraphs(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261031, 4))
	for i := range 500 {
		nodes, edges, c := randomCase(t, rng)
		g, err := newGraph(nodes, edges, c)
		require.NoError(t, err)
		r := computeCPM(g)
		t.Run(fmt.Sprintf("graf-%d", i), func(t *testing.T) { checkInvariants(t, g, r) })
		_, _, err = r.items()
		require.NoError(t, err)
	}
}

// Jadwal yang disusun maju sesuai dependensi (tanpa pelanggaran) tidak
// boleh ditandai melanggar, dan ES setiap item sama dengan mulai terjadwalnya.
func TestCPMPropertiesWithoutViolations(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	for i := range 300 {
		nodes, edges, c := randomCase(t, rng)
		for k := range nodes {
			if nodes[k].Span == nil {
				s := day(t, "2026-01-05") + rng.IntN(300)
				nodes[k].Span = &Span{Start: s, End: s + rng.IntN(10)}
			}
		}
		// Geser setiap item ke depan sampai FS terpenuhi, memakai Cascade dari setiap sumber.
		g, err := newGraph(nodes, edges, c)
		require.NoError(t, err)
		for _, idx := range g.order {
			shifts, err := Cascade(nodes, edges, c, g.nodes[idx].ID)
			require.NoError(t, err)
			for _, s := range shifts {
				for k := range nodes {
					if nodes[k].ID == s.ItemID {
						ke := s.Ke
						nodes[k].Span = &ke
					}
				}
			}
		}
		g, err = newGraph(nodes, edges, c)
		require.NoError(t, err)
		r := computeCPM(g)
		t.Run(fmt.Sprintf("graf-%d", i), func(t *testing.T) {
			checkInvariants(t, g, r)
			require.Empty(t, r.violated)
			for _, n := range g.nodes {
				require.Equal(t, n.s, n.es, n.Judul)
				require.False(t, n.violates, n.Judul)
			}
		})
	}
}

// Setelah pergeseran diterapkan, tidak ada dependensi dari item yang berubah
// atau ikut bergeser yang masih dilanggar.
func TestCascadePropertyResolvesChangedChain(t *testing.T) {
	rng := rand.New(rand.NewPCG(99, 3))
	for i := range 300 {
		nodes, edges, c := randomCase(t, rng)
		k := rng.IntN(len(nodes))
		if nodes[k].Span == nil {
			continue
		}
		delta := rng.IntN(15)
		moved := Span{Start: nodes[k].Span.Start + delta, End: nodes[k].Span.End + delta}
		nodes[k].Span = &moved
		shifts, err := Cascade(nodes, edges, c, nodes[k].ID)
		require.NoError(t, err)
		changed := map[uuid.UUID]bool{nodes[k].ID: true}
		for _, s := range shifts {
			changed[s.ItemID] = true
			for j := range nodes {
				if nodes[j].ID == s.ItemID {
					ke := s.Ke
					require.Greater(t, ke.Start, nodes[j].Span.Start, "hanya maju")
					nodes[j].Span = &ke
				}
			}
		}
		g, err := newGraph(nodes, edges, c)
		require.NoError(t, err)
		r := computeCPM(g)
		t.Run(fmt.Sprintf("graf-%d", i), func(t *testing.T) {
			for _, e := range r.violated {
				pred := g.edges[e].Pred
				// Predecessor tanpa jadwal meneruskan batasan; periksa sumbernya.
				require.False(t, changed[pred] && g.nodes[g.byID[pred]].scheduled,
					"dependensi %s>%s masih dilanggar", g.nodes[g.byID[pred]].Judul, g.nodes[g.byID[g.edges[e].Succ]].Judul)
			}
		})
	}
}

// Package demo membuat proyek contoh berisi banyak item untuk uji beban
// tampilan tabel dan Kanban (SPEC M2). Hanya untuk pengembangan.
package demo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"jwm/internal/auth"
	"jwm/internal/board"
	"jwm/internal/db"
	"jwm/internal/db/gen"
	"jwm/internal/project"
)

// Batas jumlah item dan grup agar perintah tidak dipakai mengisi database tanpa batas.
const (
	MaxItems  = 50_000
	MaxGroups = 50
)

// Options mengatur isi proyek demo.
type Options struct {
	NamaProyek string
	Items      int
	Groups     int
	// Seed membuat isi data dapat diulang; nol berarti acak.
	Seed uint64
}

// Result adalah proyek dan board yang dibuat.
type Result struct {
	ProjectID uuid.UUID
	BoardID   uuid.UUID
	Items     int
}

// ErrInvalidOptions dikembalikan untuk jumlah item atau grup di luar batas.
var ErrInvalidOptions = errors.New("jumlah item 1 sampai 50.000 dan grup 1 sampai 50")

// ErrNotAllowed dikembalikan bila pengguna tidak aktif atau tidak boleh membuat proyek.
var ErrNotAllowed = errors.New("pengguna harus aktif dan berperan Admin, PMO, atau Project Manager")

// extraColumns melengkapi kolom bawaan board (Status, Penanggung jawab)
// agar setiap tipe sel yang dapat diubah muncul di tabel.
var extraColumns = []struct {
	tipe  board.ColumnType
	judul string
}{
	{board.TypePriority, "Prioritas"},
	{board.TypeDate, "Tenggat"},
	{board.TypeTimeline, "Timeline"},
	{board.TypeProgress, "Progres"},
	{board.TypeNumber, "Estimasi (jam)"},
	{board.TypeText, "Catatan"},
}

var groupColors = []string{"biru", "hijau", "ungu", "oranye", "tosca", "merah_muda", "kuning", "abu"}

// Seed membuat proyek, board, grup, kolom, dan item demo atas nama pengguna
// dengan email tersebut. Proyek, board, grup, dan kolom lewat layanan biasa
// (dengan audit); item dan nilainya disisipkan massal lewat COPY.
func Seed(ctx context.Context, pool *pgxpool.Pool, email string, o Options) (Result, error) {
	if o.Items < 1 || o.Items > MaxItems || o.Groups < 1 || o.Groups > MaxGroups {
		return Result{}, ErrInvalidOptions
	}
	if o.NamaProyek == "" {
		o.NamaProyek = fmt.Sprintf("Demo %d item", o.Items)
	}
	q := dbgen.New(pool)
	u, err := q.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrNotAllowed
	}
	if err != nil {
		return Result{}, fmt.Errorf("cari pengguna: %w", err)
	}
	if !u.Aktif || !project.CanCreateProject(auth.Role(u.Peran)) {
		return Result{}, ErrNotAllowed
	}
	actor := auth.Principal{User: auth.User{ID: u.ID, Nama: u.Nama, Email: u.Email, Peran: auth.Role(u.Peran)}}
	meta := auth.RequestMeta{UserAgent: "seed-demo"}

	p, err := project.NewService(pool).Create(ctx, actor, project.CreateInput{Nama: o.NamaProyek}, meta)
	if err != nil {
		return Result{}, fmt.Errorf("buat proyek: %w", err)
	}
	bs := board.NewService(pool)
	b, err := bs.CreateBoard(ctx, actor, p.ID, "Pekerjaan", meta)
	if err != nil {
		return Result{}, fmt.Errorf("buat board: %w", err)
	}
	for i := len(b.Grup); i < o.Groups; i++ {
		g, err := bs.CreateGroup(ctx, actor, b.ID, fmt.Sprintf("Tahap %d", i+1), groupColors[i%len(groupColors)], meta)
		if err != nil {
			return Result{}, fmt.Errorf("buat grup: %w", err)
		}
		b.Grup = append(b.Grup, g)
	}
	for _, c := range extraColumns {
		col, err := bs.CreateColumn(ctx, actor, b.ID, c.tipe, c.judul, nil, meta)
		if err != nil {
			return Result{}, fmt.Errorf("buat kolom %s: %w", c.judul, err)
		}
		b.Kolom = append(b.Kolom, col)
	}

	seed := o.Seed
	if seed == 0 {
		seed = rand.Uint64()
	}
	g := generator{rnd: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)), userID: u.ID, today: time.Now()}
	items, values, err := g.build(b, o.Items)
	if err != nil {
		return Result{}, err
	}
	err = db.InTx(ctx, pool, func(tx pgx.Tx) error {
		qtx := q.WithTx(tx)
		if _, err := qtx.InsertDemoItems(ctx, items); err != nil {
			return fmt.Errorf("sisipkan item: %w", err)
		}
		if _, err := qtx.InsertDemoItemValues(ctx, values); err != nil {
			return fmt.Errorf("sisipkan nilai: %w", err)
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return Result{ProjectID: p.ID, BoardID: b.ID, Items: len(items)}, nil
}

type generator struct {
	rnd    *rand.Rand
	userID uuid.UUID
	today  time.Time
}

var (
	verbs   = []string{"Siapkan", "Tinjau", "Uji", "Perbarui", "Dokumentasikan", "Konfigurasi", "Migrasikan", "Audit"}
	objects = []string{"server aplikasi", "modul laporan", "jaringan kantor", "akun pengguna", "basis data", "kontrak vendor", "SOP layanan", "dasbor"}
)

// build menyusun baris item dan nilai sel. Setiap nilai dilewatkan ke
// board.NormalizeValue agar data demo sama sahnya dengan data dari API.
func (g generator) build(b board.BoardDetail, n int) ([]dbgen.InsertDemoItemsParams, []dbgen.InsertDemoItemValuesParams, error) {
	items := make([]dbgen.InsertDemoItemsParams, 0, n)
	values := make([]dbgen.InsertDemoItemValuesParams, 0, n*len(b.Kolom))
	isMember := func(id uuid.UUID) bool { return id == g.userID }
	for i := range n {
		id, err := uuid.NewV7()
		if err != nil {
			return nil, nil, fmt.Errorf("buat id item: %w", err)
		}
		grp := b.Grup[i%len(b.Grup)]
		items = append(items, dbgen.InsertDemoItemsParams{
			ID: id, BoardID: b.ID, GroupID: grp.ID,
			Judul:  fmt.Sprintf("%s %s #%d", verbs[g.rnd.IntN(len(verbs))], objects[g.rnd.IntN(len(objects))], i+1),
			Urutan: float64(i/len(b.Grup)+1) * 1024, DibuatOleh: g.userID,
		})
		for _, c := range b.Kolom {
			raw, ok := g.value(c)
			if !ok {
				continue
			}
			norm, err := board.NormalizeValue(c.Tipe, raw, board.ValueContext{Config: c.Konfigurasi, IsMember: isMember})
			if err != nil {
				return nil, nil, fmt.Errorf("nilai demo kolom %s: %w", c.Judul, err)
			}
			values = append(values, dbgen.InsertDemoItemValuesParams{ItemID: id, ColumnID: c.ID, NilaiJson: norm, DiubahOleh: &g.userID})
		}
	}
	return items, values, nil
}

// value mengembalikan nilai acak untuk kolom c; false berarti sel dibiarkan kosong.
func (g generator) value(c board.Column) (json.RawMessage, bool) {
	if g.rnd.IntN(10) == 0 {
		return nil, false // sekitar 10% sel kosong
	}
	date := func(offset int) string { return g.today.AddDate(0, 0, offset).Format("2006-01-02") }
	var v any
	switch c.Tipe {
	case board.TypeStatus:
		var cfg board.StatusConfig
		if err := json.Unmarshal(c.Konfigurasi, &cfg); err != nil || len(cfg.Labels) == 0 {
			return nil, false
		}
		v = cfg.Labels[g.rnd.IntN(len(cfg.Labels))].ID
	case board.TypePerson:
		v = []string{g.userID.String()}
	case board.TypePriority:
		v = []string{"rendah", "sedang", "tinggi", "kritis"}[g.rnd.IntN(4)]
	case board.TypeDate:
		v = date(g.rnd.IntN(120) - 30)
	case board.TypeTimeline:
		start := g.rnd.IntN(120) - 30
		v = board.Timeline{Mulai: date(start), Selesai: date(start + g.rnd.IntN(30))}
	case board.TypeProgress:
		v = g.rnd.IntN(11) * 10
	case board.TypeNumber:
		v = g.rnd.IntN(80) + 1
	case board.TypeText:
		v = fmt.Sprintf("Catatan demo %d", g.rnd.IntN(1000))
	default:
		return nil, false
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	return raw, true
}

package schedule

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"jwm/internal/audit"
	"jwm/internal/auth"
	"jwm/internal/db"
	"jwm/internal/db/gen"
	"jwm/internal/project"
)

// Galat kalender kerja.
var (
	ErrCalendarNotFound = errors.New("tanggal tidak ada di kalender kerja")
	ErrInvalidNote      = errors.New("keterangan maksimal 200 karakter")
	ErrInvalidCSV       = errors.New("berkas CSV kalender tidak valid")
	ErrInvalidYear      = errors.New("tahun harus antara 1900 dan 2999")
)

// Batas impor CSV.
const (
	maxNoteLen     = 200
	maxImportRows  = 3000
	maxImportNotes = 20 // banyaknya galat baris yang dilaporkan
)

func requireAdmin(actor auth.Principal) error {
	if !actor.User.Peran.IsAdmin() {
		return project.ErrForbidden
	}
	return nil
}

func validNote(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxNoteLen {
		return "", ErrInvalidNote
	}
	return s, nil
}

// CalendarYear adalah isi kalender satu tahun beserta tahun yang sudah terisi.
type CalendarYear struct {
	Tahun       int           `json:"tahun"`
	Items       []CalendarDay `json:"items"`
	TahunTerisi []int32       `json:"tahun_terisi"`
}

// CalendarYear mengembalikan isi kalender satu tahun. Semua pengguna login.
func (s *Service) CalendarYear(ctx context.Context, tahun int) (CalendarYear, error) {
	if tahun < 1900 || tahun > 2999 {
		return CalendarYear{}, ErrInvalidYear
	}
	rows, err := s.q.ListWorkCalendarYear(ctx, int32(tahun))
	if err != nil {
		return CalendarYear{}, fmt.Errorf("baca kalender: %w", err)
	}
	years, err := s.q.ListWorkCalendarYears(ctx)
	if err != nil {
		return CalendarYear{}, fmt.Errorf("baca tahun kalender: %w", err)
	}
	out := CalendarYear{Tahun: tahun, Items: make([]CalendarDay, len(rows)), TahunTerisi: years}
	if out.TahunTerisi == nil {
		out.TahunTerisi = []int32{}
	}
	for i, r := range rows {
		out.Items[i] = CalendarDay{Tanggal: r.Tanggal, HariKerja: r.HariKerja, Keterangan: r.Keterangan}
	}
	return out, nil
}

func recordCalendar(ctx context.Context, q *dbgen.Queries, actor auth.Principal, meta auth.RequestMeta, id, aksi string, sebelum, sesudah any) error {
	return audit.Record(ctx, q, audit.Entry{
		AktorID: &actor.User.ID, Entitas: "work_calendar", EntitasID: id, Aksi: aksi,
		Sebelum: sebelum, Sesudah: sesudah, IP: meta.IP,
	})
}

func (s *Service) previousDay(ctx context.Context, q *dbgen.Queries, tanggal string) (*CalendarDay, error) {
	r, err := q.GetWorkCalendarDay(ctx, tanggal)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("baca kalender: %w", err)
	}
	return &CalendarDay{Tanggal: r.Tanggal, HariKerja: r.HariKerja, Keterangan: r.Keterangan}, nil
}

// SetCalendarDay menambah atau mengubah satu tanggal. Hanya Admin.
func (s *Service) SetCalendarDay(ctx context.Context, actor auth.Principal, d CalendarDay, meta auth.RequestMeta) (CalendarDay, error) {
	if err := requireAdmin(actor); err != nil {
		return CalendarDay{}, err
	}
	n, err := parseDay(d.Tanggal)
	if err != nil {
		return CalendarDay{}, err
	}
	d.Tanggal = formatDay(n)
	if d.Keterangan, err = validNote(d.Keterangan); err != nil {
		return CalendarDay{}, err
	}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.LockWorkCalendar(ctx); err != nil {
			return fmt.Errorf("kunci kalender: %w", err)
		}
		before, err := s.previousDay(ctx, q, d.Tanggal)
		if err != nil {
			return err
		}
		if err := q.UpsertWorkCalendarDay(ctx, dbgen.UpsertWorkCalendarDayParams{
			Tanggal: d.Tanggal, HariKerja: d.HariKerja, Keterangan: d.Keterangan, DiubahOleh: &actor.User.ID,
		}); err != nil {
			return fmt.Errorf("simpan kalender: %w", err)
		}
		return recordCalendar(ctx, q, actor, meta, d.Tanggal, "ubah", before, d)
	})
	return d, err
}

// DeleteCalendarDay menghapus satu tanggal (kembali ke pola Senin–Jumat). Hanya Admin.
func (s *Service) DeleteCalendarDay(ctx context.Context, actor auth.Principal, tanggal string, meta auth.RequestMeta) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	n, err := parseDay(tanggal)
	if err != nil {
		return err
	}
	tanggal = formatDay(n)
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.LockWorkCalendar(ctx); err != nil {
			return fmt.Errorf("kunci kalender: %w", err)
		}
		before, err := s.previousDay(ctx, q, tanggal)
		if err != nil {
			return err
		}
		if before == nil {
			return ErrCalendarNotFound
		}
		if _, err := q.DeleteWorkCalendarDay(ctx, tanggal); err != nil {
			return fmt.Errorf("hapus kalender: %w", err)
		}
		return recordCalendar(ctx, q, actor, meta, tanggal, "hapus", before, nil)
	})
}

// ImportResult adalah ringkasan impor CSV.
type ImportResult struct {
	Jumlah  int   `json:"jumlah"`
	Tahun   []int `json:"tahun"`
	Dihapus int64 `json:"dihapus"`
}

// ParseCalendarCSV membaca CSV kalender kerja. Baris pertama wajib header
// berisi kolom tanggal (YYYY-MM-DD), dan opsional keterangan serta hari_kerja,
// dalam urutan apa pun. Pemisah koma atau titik koma. hari_kerja kosong berarti
// libur; nilai "ya", "1", "true", atau "kerja" berarti hari kerja.
// Semua galat dikumpulkan dengan nomor baris; berkas ditolak seluruhnya.
func ParseCalendarCSV(data []byte) ([]CalendarDay, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // BOM UTF-8 dari Excel
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%w: berkas harus berenkode UTF-8", ErrInvalidCSV)
	}
	first, _, _ := bytes.Cut(data, []byte("\n"))
	r := csv.NewReader(bytes.NewReader(data))
	if bytes.Count(first, []byte(";")) > bytes.Count(first, []byte(",")) {
		r.Comma = ';'
	}
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: berkas kosong", ErrInvalidCSV)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCSV, err)
	}
	col := map[string]int{}
	for i, h := range header {
		name := strings.ToLower(strings.TrimSpace(h))
		if _, dup := col[name]; dup {
			return nil, fmt.Errorf("%w: kolom %q muncul dua kali di header", ErrInvalidCSV, name)
		}
		col[name] = i
	}
	iTanggal, ok := col["tanggal"]
	if !ok {
		return nil, fmt.Errorf("%w: header wajib berisi kolom tanggal (opsional: keterangan, hari_kerja)", ErrInvalidCSV)
	}
	for name := range col {
		if name != "tanggal" && name != "keterangan" && name != "hari_kerja" {
			return nil, fmt.Errorf("%w: kolom %q tidak dikenal; gunakan tanggal, keterangan, hari_kerja", ErrInvalidCSV, name)
		}
	}
	field := func(rec []string, name string) string {
		if i, ok := col[name]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}

	var out []CalendarDay
	var problems []string
	seen := map[string]int{}
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				problems = append(problems, fmt.Sprintf("baris %d: format CSV tidak valid (%v)", pe.Line, pe.Err))
			} else {
				problems = append(problems, err.Error())
			}
			break
		}
		line, _ := r.FieldPos(0)
		if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
			continue // baris kosong
		}
		if len(out)+len(problems) >= maxImportRows {
			return nil, fmt.Errorf("%w: maksimal %d baris per impor", ErrInvalidCSV, maxImportRows)
		}
		bad := func(msg string, a ...any) {
			problems = append(problems, fmt.Sprintf("baris %d: ", line)+fmt.Sprintf(msg, a...))
		}
		raw := ""
		if iTanggal < len(rec) {
			raw = strings.TrimSpace(rec[iTanggal])
		}
		n, err := parseDay(raw)
		if err != nil {
			bad("tanggal %q harus berformat YYYY-MM-DD", raw)
			continue
		}
		tanggal := formatDay(n)
		if prev, dup := seen[tanggal]; dup {
			bad("tanggal %s sudah ada di baris %d", tanggal, prev)
			continue
		}
		seen[tanggal] = line
		note, err := validNote(field(rec, "keterangan"))
		if err != nil {
			bad("keterangan maksimal %d karakter", maxNoteLen)
			continue
		}
		var kerja bool
		switch v := strings.ToLower(field(rec, "hari_kerja")); v {
		case "", "0", "tidak", "false", "libur":
		case "1", "ya", "true", "kerja":
			kerja = true
		default:
			bad("hari_kerja %q tidak dikenal; gunakan ya atau tidak", v)
			continue
		}
		out = append(out, CalendarDay{Tanggal: tanggal, HariKerja: kerja, Keterangan: note})
	}
	if len(problems) > 0 {
		more := ""
		if len(problems) > maxImportNotes {
			more = fmt.Sprintf("; dan %d galat lain", len(problems)-maxImportNotes)
			problems = problems[:maxImportNotes]
		}
		return nil, fmt.Errorf("%w: %s%s", ErrInvalidCSV, strings.Join(problems, "; "), more)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: tidak ada baris data", ErrInvalidCSV)
	}
	return out, nil
}

// ImportCalendar menyimpan isi CSV dalam satu transaksi. ganti = true menghapus
// lebih dulu semua tanggal pada tahun-tahun yang ada di berkas, sehingga
// kalender tahun itu sama persis dengan berkas. Hanya Admin.
func (s *Service) ImportCalendar(ctx context.Context, actor auth.Principal, data []byte, ganti bool, meta auth.RequestMeta) (ImportResult, error) {
	if err := requireAdmin(actor); err != nil {
		return ImportResult{}, err
	}
	days, err := ParseCalendarCSV(data)
	if err != nil {
		return ImportResult{}, err
	}
	var years []int
	for _, d := range days {
		n, _ := parseDay(d.Tanggal)
		if y := yearOf(n); !slices.Contains(years, y) {
			years = append(years, y)
		}
	}
	slices.Sort(years)
	res := ImportResult{Jumlah: len(days), Tahun: years}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.LockWorkCalendar(ctx); err != nil {
			return fmt.Errorf("kunci kalender: %w", err)
		}
		if ganti {
			ys := make([]int32, len(years))
			for i, y := range years {
				ys[i] = int32(y)
			}
			n, err := q.DeleteWorkCalendarYears(ctx, ys)
			if err != nil {
				return fmt.Errorf("hapus kalender lama: %w", err)
			}
			res.Dihapus = n
		}
		for _, d := range days {
			if err := q.UpsertWorkCalendarDay(ctx, dbgen.UpsertWorkCalendarDayParams{
				Tanggal: d.Tanggal, HariKerja: d.HariKerja, Keterangan: d.Keterangan, DiubahOleh: &actor.User.ID,
			}); err != nil {
				return fmt.Errorf("simpan kalender %s: %w", d.Tanggal, err)
			}
		}
		return recordCalendar(ctx, q, actor, meta, "", "impor", nil,
			map[string]any{"jumlah": res.Jumlah, "tahun": years, "ganti": ganti, "dihapus": res.Dihapus})
	})
	return res, err
}

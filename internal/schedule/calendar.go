package schedule

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Tanggal diperlakukan sebagai tanggal murni: string YYYY-MM-DD dari API dan
// database diubah ke nomor hari sejak 1970-01-01 tanpa jam dan zona waktu.
const dateLayout = "2006-01-02"

// ErrInvalidDate dipakai bila tanggal tidak berformat YYYY-MM-DD yang valid.
var ErrInvalidDate = errors.New("tanggal harus berformat YYYY-MM-DD yang valid")

// parseDay mengubah YYYY-MM-DD menjadi nomor hari.
func parseDay(s string) (int, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidDate, s)
	}
	return int(t.Unix() / 86400), nil
}

// formatDay mengubah nomor hari menjadi YYYY-MM-DD.
func formatDay(day int) string {
	return time.Unix(int64(day)*86400, 0).UTC().Format(dateLayout)
}

func yearOf(day int) int {
	return time.Unix(int64(day)*86400, 0).UTC().Year()
}

// CalendarDay adalah satu baris work_calendar.
type CalendarDay struct {
	Tanggal    string `json:"tanggal"`
	HariKerja  bool   `json:"hari_kerja"`
	Keterangan string `json:"keterangan"`
}

// Calendar menentukan hari kerja: Senin sampai Jumat, kecuali ditimpa baris
// work_calendar (libur pada hari kerja, atau hari kerja pada akhir pekan).
type Calendar struct {
	override map[int]bool
	years    map[int]bool
}

// NewCalendar membangun kalender dari baris work_calendar.
func NewCalendar(days []CalendarDay) (*Calendar, error) {
	c := &Calendar{override: make(map[int]bool, len(days)), years: map[int]bool{}}
	for _, d := range days {
		n, err := parseDay(d.Tanggal)
		if err != nil {
			return nil, err
		}
		c.override[n] = d.HariKerja
		c.years[yearOf(n)] = true
	}
	return c, nil
}

// IsWorkday melaporkan apakah day adalah hari kerja.
func (c *Calendar) IsWorkday(day int) bool {
	if w, ok := c.override[day]; ok {
		return w
	}
	// 1970-01-01 adalah Kamis; (day+4)%7 memberi 0 = Minggu … 6 = Sabtu.
	wd := ((day+4)%7 + 7) % 7
	return wd != 0 && wd != 6
}

// HasYear melaporkan apakah Admin sudah mengisi kalender untuk tahun itu.
func (c *Calendar) HasYear(year int) bool { return c.years[year] }

// Batas perluasan indeks hari kerja, untuk mencegah perulangan tanpa akhir
// bila data tidak wajar. Lebih dari cukup untuk lag ±365 hari kerja berantai.
const (
	indexChunk    = 400
	indexMaxRange = 366 * 60
)

var errIndexRange = errors.New("jadwal melewati batas rentang kalender yang didukung")

// workIndex memetakan tanggal ke titik hari kerja dan sebaliknya.
//
// Titik p adalah batas di awal hari kerja ke-p (relatif terhadap anchor).
// point(d) = banyaknya hari kerja sebelum d, sehingga item [mulai, selesai]
// menempati titik [point(mulai), point(selesai+1)) dan durasinya adalah
// selisih keduanya dalam hari kerja. Item yang mulai pada hari libur
// otomatis dianggap mulai pada hari kerja berikutnya.
type workIndex struct {
	cal    *Calendar
	anchor int
	lo, hi int   // rentang hari yang sudah dipindai [lo, hi)
	days   []int // hari kerja dalam [lo, hi), terurut
}

func newWorkIndex(cal *Calendar, anchor int) *workIndex {
	w := &workIndex{cal: cal, anchor: anchor, lo: anchor, hi: anchor}
	return w
}

func (w *workIndex) extendHi(to int) error {
	if to-w.lo > indexMaxRange {
		return errIndexRange
	}
	for d := w.hi; d < to; d++ {
		if w.cal.IsWorkday(d) {
			w.days = append(w.days, d)
		}
	}
	w.hi = to
	return nil
}

func (w *workIndex) extendLo(to int) error {
	if w.hi-to > indexMaxRange {
		return errIndexRange
	}
	var add []int
	for d := to; d < w.lo; d++ {
		if w.cal.IsWorkday(d) {
			add = append(add, d)
		}
	}
	w.days = append(add, w.days...)
	w.lo = to
	return nil
}

func (w *workIndex) cover(day int) error {
	if day < w.lo {
		if err := w.extendLo(day - indexChunk); err != nil {
			return err
		}
	}
	if day >= w.hi {
		return w.extendHi(day + indexChunk)
	}
	return nil
}

// point memberi banyaknya hari kerja sebelum day, relatif terhadap anchor.
func (w *workIndex) point(day int) (int, error) {
	if err := w.cover(day); err != nil {
		return 0, err
	}
	if err := w.cover(w.anchor); err != nil {
		return 0, err
	}
	return sort.SearchInts(w.days, day) - sort.SearchInts(w.days, w.anchor), nil
}

// dayAt memberi tanggal hari kerja yang dimulai di titik p.
func (w *workIndex) dayAt(p int) (int, error) {
	base := sort.SearchInts(w.days, w.anchor)
	for base+p < 0 {
		if err := w.extendLo(w.lo - indexChunk); err != nil {
			return 0, err
		}
		base = sort.SearchInts(w.days, w.anchor)
	}
	for base+p >= len(w.days) {
		if err := w.extendHi(w.hi + indexChunk); err != nil {
			return 0, err
		}
	}
	return w.days[base+p], nil
}

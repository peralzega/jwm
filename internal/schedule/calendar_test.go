package schedule

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func day(t *testing.T, s string) int {
	t.Helper()
	d, err := parseDay(s)
	require.NoError(t, err)
	return d
}

func cal(t *testing.T, days ...CalendarDay) *Calendar {
	t.Helper()
	c, err := NewCalendar(days)
	require.NoError(t, err)
	return c
}

func TestParseDayRoundTrip(t *testing.T) {
	for _, s := range []string{
		"1970-01-01", "1969-12-31", "2026-01-31", "2026-02-01", "2026-12-31", "2027-01-01",
		"2028-02-28", "2028-02-29", "2028-03-01", "2100-02-28", "2100-03-01",
	} {
		t.Run(s, func(t *testing.T) {
			assert.Equal(t, s, formatDay(day(t, s)))
		})
	}
	// Batas bulan dan tahun berurutan satu hari, termasuk tahun kabisat.
	for _, pair := range [][2]string{
		{"2026-01-31", "2026-02-01"}, {"2026-02-28", "2026-03-01"}, {"2028-02-28", "2028-02-29"},
		{"2028-02-29", "2028-03-01"}, {"2026-12-31", "2027-01-01"}, {"2100-02-28", "2100-03-01"},
	} {
		assert.Equal(t, 1, day(t, pair[1])-day(t, pair[0]), pair)
	}
	for _, bad := range []string{"", "2026-02-30", "2027-02-29", "2026-13-01", "31-12-2026", "2026-1-5", "2026-01-05T00:00:00Z"} {
		_, err := parseDay(bad)
		assert.ErrorIs(t, err, ErrInvalidDate, bad)
	}
}

func TestCalendarWorkdays(t *testing.T) {
	c := cal(t,
		CalendarDay{Tanggal: "2026-11-04", HariKerja: false, Keterangan: "Libur"},
		CalendarDay{Tanggal: "2026-11-07", HariKerja: true, Keterangan: "Sabtu masuk"},
	)
	tests := []struct {
		tanggal string
		kerja   bool
	}{
		{"2026-11-01", false}, // Minggu
		{"2026-11-02", true},  // Senin
		{"2026-11-04", false}, // Rabu, libur dari kalender
		{"2026-11-06", true},  // Jumat
		{"2026-11-07", true},  // Sabtu, ditimpa menjadi hari kerja
		{"2026-11-08", false}, // Minggu
		{"1969-12-31", true},  // Rabu sebelum epoch
		{"1969-12-28", false}, // Minggu sebelum epoch
		{"2028-02-29", true},  // Selasa kabisat
	}
	for _, tt := range tests {
		t.Run(tt.tanggal, func(t *testing.T) {
			assert.Equal(t, tt.kerja, c.IsWorkday(day(t, tt.tanggal)))
		})
	}
	assert.True(t, c.HasYear(2026))
	assert.False(t, c.HasYear(2027))
}

func TestWorkIndex(t *testing.T) {
	c := cal(t, CalendarDay{Tanggal: "2027-01-01", Keterangan: "Tahun baru"})
	w := newWorkIndex(c, day(t, "2026-12-28"))
	p := func(s string) int {
		v, err := w.point(day(t, s))
		require.NoError(t, err)
		return v
	}
	at := func(n int) string {
		d, err := w.dayAt(n)
		require.NoError(t, err)
		return formatDay(d)
	}
	// Senin 28 Des sampai Kamis 31 Des = 4 hari kerja; Jumat 1 Jan libur; Senin 4 Jan.
	assert.Equal(t, 0, p("2026-12-28"))
	assert.Equal(t, 4, p("2027-01-01"))
	assert.Equal(t, 4, p("2027-01-02"))
	assert.Equal(t, 4, p("2027-01-04"))
	assert.Equal(t, 5, p("2027-01-05"))
	assert.Equal(t, "2027-01-04", at(4))
	assert.Equal(t, "2026-12-31", at(3))
	// Mundur melewati anchor dan perluasan indeks.
	assert.Equal(t, 0, p("2026-12-27")) // Minggu: tidak ada hari kerja di [27, 28)
	assert.Equal(t, -1, p("2026-12-25"))
	assert.Equal(t, -5, p("2026-12-21"))
	assert.Equal(t, "2026-12-25", at(-1))
	assert.Equal(t, "2025-12-29", at(-52*5)) // tepat 52 minggu sebelumnya
	// Jauh ke depan memicu perluasan berulang.
	assert.Equal(t, "2028-02-29", at(p("2028-02-29")))
	assert.Equal(t, p("2028-03-01")-1, p("2028-02-29"))
}

func TestWorkIndexRangeLimit(t *testing.T) {
	w := newWorkIndex(cal(t), 0)
	_, err := w.dayAt(366 * 60)
	assert.ErrorIs(t, err, errIndexRange)
	_, err = w.point(-366 * 61)
	assert.ErrorIs(t, err, errIndexRange)
}

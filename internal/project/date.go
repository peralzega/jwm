package project

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

const dateLayout = "2006-01-02"

// Date adalah tanggal kalender tanpa jam, berformat JSON "YYYY-MM-DD".
type Date struct{ time.Time }

// MarshalJSON menulis "YYYY-MM-DD".
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.Format(dateLayout)) }

// UnmarshalJSON membaca "YYYY-MM-DD".
func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("tanggal harus string YYYY-MM-DD: %w", err)
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return fmt.Errorf("tanggal harus berformat YYYY-MM-DD: %w", err)
	}
	d.Time = t
	return nil
}

func toPgDate(d *Date) pgtype.Date {
	if d == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: d.Time, Valid: true}
}

func fromPgDate(p pgtype.Date) *Date {
	if !p.Valid {
		return nil
	}
	return &Date{p.Time}
}

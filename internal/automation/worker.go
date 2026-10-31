package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"jwm/internal/audit"
	"jwm/internal/board"
	"jwm/internal/db/gen"
	"jwm/internal/schedule"
)

// Hasil eksekusi aturan (automation_runs.hasil).
const (
	RunOK      = "berhasil"
	RunSkipped = "dilewati"
	RunFailed  = "gagal"
	RunBlocked = "diblokir_loop"
)

// Bawaan worker.
const (
	DefaultPoll     = time.Second
	DefaultDueEvery = 15 * time.Minute
	batchSize       = 50
	// positionStep sama dengan jarak urutan item di paket board.
	positionStep = 1024
)

// Worker menjalankan antrean kejadian otomasi dan penjadwal "tanggal tiba".
type Worker struct {
	pool   *pgxpool.Pool
	q      *dbgen.Queries
	log    *slog.Logger
	limits Limits
	// Poll dan DueEvery dapat diubah tes.
	Poll     time.Duration
	DueEvery time.Duration
	// Now sumber waktu. ASSUMPTION: "hari ini" = tanggal lokal server (WIB).
	Now func() time.Time
}

// NewWorker membuat worker otomasi.
func NewWorker(pool *pgxpool.Pool, log *slog.Logger, limits Limits) *Worker {
	return &Worker{pool: pool, q: dbgen.New(pool), log: log, limits: limits, Poll: DefaultPoll, DueEvery: DefaultDueEvery, Now: time.Now}
}

// Run memproses antrean sampai ctx selesai.
func (w *Worker) Run(ctx context.Context) {
	poll := time.NewTicker(w.Poll)
	defer poll.Stop()
	due := time.NewTicker(w.DueEvery)
	defer due.Stop()
	if err := w.EnqueueDue(ctx); err != nil && ctx.Err() == nil {
		w.log.Error("otomasi: penjadwal tanggal", "err", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-due.C:
			if err := w.EnqueueDue(ctx); err != nil && ctx.Err() == nil {
				w.log.Error("otomasi: penjadwal tanggal", "err", err)
			}
		case <-poll.C:
			// Habiskan antrean sebelum menunggu tick berikutnya.
			for ctx.Err() == nil {
				n, err := w.ProcessBatch(ctx)
				if err != nil {
					if ctx.Err() == nil {
						w.log.Error("otomasi: proses antrean", "err", err)
					}
					break
				}
				if n < batchSize {
					break
				}
			}
		}
	}
}

// EnqueueDue membuat kejadian "tanggal tiba" untuk item yang tanggalnya hari ini.
func (w *Worker) EnqueueDue(ctx context.Context) error {
	rules, err := w.q.ListDueDateRules(ctx)
	if err != nil {
		return fmt.Errorf("daftar aturan tanggal: %w", err)
	}
	today := schedule.FormatDay(schedule.DayOf(w.Now()))
	for _, r := range rules {
		var t Trigger
		if json.Unmarshal(r.PemicuJson, &t) != nil || t.ColumnID == nil {
			continue
		}
		id := r.ID
		if _, err := w.q.EnqueueDueDateEvents(ctx, dbgen.EnqueueDueDateEventsParams{
			BoardID: r.BoardID, ColumnID: t.ColumnID, RuleID: &id, HariIni: today,
		}); err != nil {
			return fmt.Errorf("kejadian tanggal tiba: %w", err)
		}
	}
	return nil
}

// ProcessBatch memproses paling banyak satu batch kejadian dalam satu transaksi.
// Mengembalikan jumlah kejadian yang diproses.
func (w *Worker) ProcessBatch(ctx context.Context) (int, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("mulai transaksi: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := w.q.WithTx(tx)
	events, err := q.ClaimAutomationEvents(ctx, batchSize)
	if err != nil {
		return 0, fmt.Errorf("ambil antrean: %w", err)
	}
	rulesByBoard := map[uuid.UUID][]dbgen.AutomationRule{}
	for _, row := range events {
		rules, ok := rulesByBoard[row.BoardID]
		if !ok {
			if rules, err = q.ListActiveRulesForBoard(ctx, row.BoardID); err != nil {
				return 0, fmt.Errorf("daftar aturan: %w", err)
			}
			rulesByBoard[row.BoardID] = rules
		}
		if err := w.process(ctx, tx, q, toEvent(row), rules); err != nil {
			return 0, err
		}
		if err := q.MarkAutomationEventDone(ctx, row.ID); err != nil {
			return 0, fmt.Errorf("tandai kejadian: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return len(events), nil
}

func toEvent(r dbgen.AutomationEvent) Event {
	e := Event{
		ID: r.ID, BoardID: r.BoardID, ItemID: r.ItemID, Jenis: r.Jenis, ColumnID: r.ColumnID, RuleID: r.RuleID,
		Sebelum: r.Sebelum, Sesudah: r.Sesudah,
	}
	_ = json.Unmarshal(r.Rantai, &e.Rantai)
	return e
}

// process menjalankan semua aturan yang cocok untuk satu kejadian. Galat aksi
// dicatat sebagai run "gagal" (savepoint dibatalkan); galat lain menghentikan batch.
func (w *Worker) process(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, e Event, rules []dbgen.AutomationRule) error {
	type matched struct {
		rule dbgen.AutomationRule
		cond []Condition
		acts []Action
	}
	var todo []matched
	for _, r := range rules {
		var t Trigger
		if json.Unmarshal(r.PemicuJson, &t) != nil || !t.Matches(e, r.ID) {
			continue
		}
		m := matched{rule: r}
		if json.Unmarshal(r.KondisiJson, &m.cond) != nil || json.Unmarshal(r.AksiJson, &m.acts) != nil {
			if err := w.logRun(ctx, q, r.ID, e, RunFailed, "isi aturan rusak"); err != nil {
				return err
			}
			continue
		}
		todo = append(todo, m)
	}
	if len(todo) == 0 {
		return nil
	}

	item, err := q.GetItemForAutomation(ctx, e.ItemID)
	skip := ""
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		skip = "item sudah dihapus"
	case err != nil:
		return fmt.Errorf("baca item: %w", err)
	case item.DiarsipkanPada != nil:
		skip = "proyek diarsipkan"
	}
	if skip != "" {
		for _, m := range todo {
			if err := w.logRun(ctx, q, m.rule.ID, e, RunSkipped, skip); err != nil {
				return err
			}
		}
		return nil
	}
	values, err := q.ListItemValuesForItem(ctx, e.ItemID)
	if err != nil {
		return fmt.Errorf("baca nilai item: %w", err)
	}
	state := ItemState{GroupID: item.GroupID, Values: map[uuid.UUID]json.RawMessage{}}
	for _, v := range values {
		state.Values[v.ColumnID] = v.NilaiJson
	}

	for _, m := range todo {
		if blocked, why := w.limits.LoopBlocked(e.Rantai, m.rule.ID); blocked {
			if err := w.logRun(ctx, q, m.rule.ID, e, RunBlocked, why); err != nil {
				return err
			}
			continue
		}
		if ok, why := ConditionsMet(m.cond, state); !ok {
			if err := w.logRun(ctx, q, m.rule.ID, e, RunSkipped, why); err != nil {
				return err
			}
			continue
		}
		next, msg, err := w.apply(ctx, tx, item, state, m.rule, m.acts, append(append([]uuid.UUID{}, e.Rantai...), m.rule.ID))
		hasil := RunOK
		if err != nil {
			hasil, msg = RunFailed, err.Error()
		} else {
			state = next
		}
		if err := w.logRun(ctx, q, m.rule.ID, e, hasil, msg); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) logRun(ctx context.Context, q *dbgen.Queries, ruleID uuid.UUID, e Event, hasil, pesan string) error {
	item, ev := e.ItemID, e.ID
	if err := q.InsertAutomationRun(ctx, dbgen.InsertAutomationRunParams{
		RuleID: ruleID, ItemID: &item, EventID: &ev, Hasil: hasil, Pesan: pesan,
	}); err != nil {
		return fmt.Errorf("catat eksekusi: %w", err)
	}
	return nil
}

// apply menjalankan aksi aturan dalam savepoint. Rantai disetel agar kejadian
// yang ditimbulkan aksi membawa jejak aturan ini (pencegahan loop).
func (w *Worker) apply(ctx context.Context, tx pgx.Tx, item dbgen.GetItemForAutomationRow, state ItemState,
	rule dbgen.AutomationRule, acts []Action, rantai []uuid.UUID) (ItemState, string, error) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return state, "", fmt.Errorf("savepoint: %w", err)
	}
	defer func() { _ = sp.Rollback(ctx) }()
	q := w.q.WithTx(sp)
	chain, _ := json.Marshal(rantai)
	if err := q.SetAutomationChain(ctx, string(chain)); err != nil {
		return state, "", fmt.Errorf("setel rantai: %w", err)
	}
	next := ItemState{GroupID: state.GroupID, Values: map[uuid.UUID]json.RawMessage{}}
	for k, v := range state.Values {
		next.Values[k] = v
	}
	origin := map[string]any{"rule_id": rule.ID, "nama": rule.Nama, "dibuat_oleh": rule.DibuatOleh}
	var done []string
	for i, a := range acts {
		changed, err := w.act(ctx, q, item, &next, a, origin)
		if err != nil {
			return state, "", fmt.Errorf("aksi %d: %w", i+1, err)
		}
		if changed != "" {
			done = append(done, changed)
		}
	}
	if err := sp.Commit(ctx); err != nil {
		return state, "", fmt.Errorf("simpan savepoint: %w", err)
	}
	if len(done) == 0 {
		return next, "tidak ada perubahan (nilai sudah sesuai)", nil
	}
	return next, strings.Join(done, "; "), nil
}

// act menjalankan satu aksi sebagai sistem (aktor audit kosong, asal aturan
// dicatat). Mengembalikan ringkasan perubahan, atau "" bila tidak ada perubahan.
func (w *Worker) act(ctx context.Context, q *dbgen.Queries, item dbgen.GetItemForAutomationRow, st *ItemState, a Action, origin map[string]any) (string, error) {
	rec := func(aksi string, sebelum, sesudah map[string]any) error {
		sesudah["otomasi"] = origin
		return audit.Record(ctx, q, audit.Entry{
			Entitas: "item", EntitasID: item.ID.String(), Aksi: aksi, Sebelum: sebelum, Sesudah: sesudah,
		})
	}
	switch a.Jenis {
	case ActionMoveGroup:
		if a.GroupID == nil {
			return "", errors.New("grup tujuan kosong")
		}
		if *a.GroupID == st.GroupID {
			return "", nil
		}
		g, err := q.GetGroupForShare(ctx, *a.GroupID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && g.BoardID != item.BoardID) {
			return "", errors.New("grup tujuan sudah dihapus")
		}
		if err != nil {
			return "", fmt.Errorf("kunci grup: %w", err)
		}
		last, err := q.MaxItemUrutan(ctx, g.ID)
		if err != nil {
			return "", fmt.Errorf("urutan item: %w", err)
		}
		if err := q.MoveItem(ctx, dbgen.MoveItemParams{ID: item.ID, GroupID: g.ID, Urutan: last + positionStep}); err != nil {
			return "", fmt.Errorf("pindah item: %w", err)
		}
		if err := rec("pindah", map[string]any{"group_id": st.GroupID}, map[string]any{"group_id": g.ID}); err != nil {
			return "", err
		}
		st.GroupID = g.ID
		return fmt.Sprintf("dipindah ke grup %q", g.Judul), nil

	case ActionSetValue:
		if a.ColumnID == nil {
			return "", errors.New("kolom kosong")
		}
		col, err := q.GetColumnWithProject(ctx, *a.ColumnID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && col.BoardColumn.BoardID != item.BoardID) {
			return "", errors.New("kolom sudah dihapus")
		}
		if err != nil {
			return "", fmt.Errorf("baca kolom: %w", err)
		}
		tipe := board.ColumnType(col.BoardColumn.Tipe)
		before := st.Values[col.BoardColumn.ID]
		var after json.RawMessage
		if s := strings.TrimSpace(string(a.Nilai)); s != "" && s != "null" {
			members, err := q.ListProjectMemberIDs(ctx, item.ProjectID)
			if err != nil {
				return "", fmt.Errorf("baca anggota: %w", err)
			}
			set := map[uuid.UUID]bool{}
			for _, m := range members {
				set[m] = true
			}
			// Dinormalkan ulang: label status atau keanggotaan bisa berubah sejak aturan dibuat.
			after, err = board.NormalizeValue(tipe, a.Nilai, board.ValueContext{
				Config: col.BoardColumn.KonfigurasiJson, IsMember: func(id uuid.UUID) bool { return set[id] },
			})
			if err != nil {
				return "", err
			}
		}
		if sameJSON(before, after) {
			return "", nil
		}
		if after == nil {
			err = q.DeleteItemValue(ctx, dbgen.DeleteItemValueParams{ItemID: item.ID, ColumnID: col.BoardColumn.ID})
		} else {
			err = q.UpsertItemValue(ctx, dbgen.UpsertItemValueParams{ItemID: item.ID, ColumnID: col.BoardColumn.ID, NilaiJson: after})
		}
		if err != nil {
			return "", fmt.Errorf("simpan nilai: %w", err)
		}
		if err := q.TouchItem(ctx, item.ID); err != nil {
			return "", fmt.Errorf("perbarui item: %w", err)
		}
		if err := rec("ubah_nilai",
			map[string]any{"column_id": col.BoardColumn.ID, "tipe": tipe, "nilai": before},
			map[string]any{"column_id": col.BoardColumn.ID, "tipe": tipe, "nilai": after}); err != nil {
			return "", err
		}
		if after == nil {
			delete(st.Values, col.BoardColumn.ID)
		} else {
			st.Values[col.BoardColumn.ID] = after
		}
		return fmt.Sprintf("kolom %q diubah", col.BoardColumn.Judul), nil
	}
	return "", fmt.Errorf("aksi %q tidak dikenal", a.Jenis)
}

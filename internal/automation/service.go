package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"jwm/internal/audit"
	"jwm/internal/auth"
	"jwm/internal/board"
	"jwm/internal/db"
	"jwm/internal/db/gen"
	"jwm/internal/project"
)

// Batas aturan (keputusan M7).
const (
	MaxRulesPerBoard = 50
	MaxActions       = 10
	MaxConditions    = 10
	MaxRunsListed    = 100
)

// Galat layanan aturan.
var (
	ErrNotFound     = errors.New("aturan otomasi tidak ditemukan")
	ErrInvalidRule  = errors.New("aturan otomasi tidak valid")
	ErrTooManyRules = fmt.Errorf("satu board paling banyak memiliki %d aturan otomasi", MaxRulesPerBoard)
)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRule, fmt.Sprintf(format, a...))
}

// Rule adalah tampilan aturan.
type Rule struct {
	ID             uuid.UUID   `json:"id"`
	BoardID        uuid.UUID   `json:"board_id"`
	Nama           string      `json:"nama"`
	Pemicu         Trigger     `json:"pemicu"`
	Kondisi        []Condition `json:"kondisi"`
	Aksi           []Action    `json:"aksi"`
	Aktif          bool        `json:"aktif"`
	DibuatOlehNama string      `json:"dibuat_oleh_nama,omitempty"`
	DibuatPada     time.Time   `json:"dibuat_pada"`
	DiubahPada     time.Time   `json:"diubah_pada"`
	// WebhookRahasia hanya diisi sekali: saat rahasia HMAC dibuat atau diganti.
	WebhookRahasia *string `json:"webhook_rahasia,omitempty"`
}

// RuleInput adalah isi aturan baru atau perubahan.
type RuleInput struct {
	Nama    string      `json:"nama"`
	Pemicu  Trigger     `json:"pemicu"`
	Kondisi []Condition `json:"kondisi"`
	Aksi    []Action    `json:"aksi"`
	Aktif   bool        `json:"aktif"`
}

// Service mengelola aturan otomasi.
type Service struct {
	pool    *pgxpool.Pool
	q       *dbgen.Queries
	webhook WebhookConfig
}

// NewService membuat layanan aturan otomasi.
func NewService(pool *pgxpool.Pool, webhook WebhookConfig) *Service {
	return &Service{pool: pool, q: dbgen.New(pool), webhook: webhook}
}

func toRule(r dbgen.AutomationRule, creator string) (Rule, error) {
	out := Rule{
		ID: r.ID, BoardID: r.BoardID, Nama: r.Nama, Aktif: r.Aktif, DibuatOlehNama: creator,
		DibuatPada: r.DibuatPada, DiubahPada: r.DiubahPada, Kondisi: []Condition{}, Aksi: []Action{},
	}
	if err := json.Unmarshal(r.PemicuJson, &out.Pemicu); err != nil {
		return out, fmt.Errorf("baca pemicu: %w", err)
	}
	if err := json.Unmarshal(r.KondisiJson, &out.Kondisi); err != nil {
		return out, fmt.Errorf("baca kondisi: %w", err)
	}
	if err := json.Unmarshal(r.AksiJson, &out.Aksi); err != nil {
		return out, fmt.Errorf("baca aksi: %w", err)
	}
	return out, nil
}

// boardSchema adalah kolom, grup, dan anggota board untuk validasi aturan.
type boardSchema struct {
	cols    map[uuid.UUID]dbgen.BoardColumn
	groups  map[uuid.UUID]bool
	members map[uuid.UUID]bool
}

func loadSchema(ctx context.Context, q *dbgen.Queries, boardID, projectID uuid.UUID) (boardSchema, error) {
	s := boardSchema{cols: map[uuid.UUID]dbgen.BoardColumn{}, groups: map[uuid.UUID]bool{}, members: map[uuid.UUID]bool{}}
	cols, err := q.ListColumns(ctx, boardID)
	if err != nil {
		return s, fmt.Errorf("daftar kolom: %w", err)
	}
	for _, c := range cols {
		s.cols[c.ID] = c
	}
	groups, err := q.ListGroups(ctx, boardID)
	if err != nil {
		return s, fmt.Errorf("daftar grup: %w", err)
	}
	for _, g := range groups {
		s.groups[g.ID] = true
	}
	members, err := q.ListProjectMemberIDs(ctx, projectID)
	if err != nil {
		return s, fmt.Errorf("daftar anggota: %w", err)
	}
	for _, m := range members {
		s.members[m] = true
	}
	return s, nil
}

func (s boardSchema) column(id *uuid.UUID, what string, types ...board.ColumnType) (dbgen.BoardColumn, error) {
	if id == nil {
		return dbgen.BoardColumn{}, invalid("%s: kolom wajib dipilih", what)
	}
	c, ok := s.cols[*id]
	if !ok || !slices.Contains(types, board.ColumnType(c.Tipe)) {
		return c, invalid("%s: kolom tidak ada di board ini atau tipenya tidak sesuai", what)
	}
	return c, nil
}

func (s boardSchema) hasLabel(colID *uuid.UUID, id string) bool {
	if id == "" {
		return true // "" = status kosong
	}
	for _, c := range s.cols {
		if board.ColumnType(c.Tipe) != board.TypeStatus || (colID != nil && c.ID != *colID) {
			continue
		}
		var cfg board.StatusConfig
		_ = json.Unmarshal(c.KonfigurasiJson, &cfg)
		for _, l := range cfg.Labels {
			if l.ID == id {
				return true
			}
		}
	}
	return false
}

// normalize memvalidasi nilai untuk kolom c (null diizinkan = kosongkan).
func (s boardSchema) normalize(c dbgen.BoardColumn, raw json.RawMessage, what string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return json.RawMessage("null"), nil
	}
	out, err := board.NormalizeValue(board.ColumnType(c.Tipe), raw, board.ValueContext{
		Config: c.KonfigurasiJson, IsMember: func(id uuid.UUID) bool { return s.members[id] },
	})
	if err != nil {
		return nil, invalid("%s: %v", what, err)
	}
	return out, nil
}

// Tipe kolom yang boleh diubah aksi dan dibandingkan kondisi. Timeline tidak
// termasuk karena perubahan timeline menggeser successor (M4) atas nama pengguna.
var valueTypes = []board.ColumnType{
	board.TypeStatus, board.TypePriority, board.TypePerson, board.TypeText,
	board.TypeNumber, board.TypeProgress, board.TypeDate,
}

// validate memeriksa dan menormalkan aturan terhadap skema board.
func (s boardSchema) validate(in RuleInput, webhook WebhookConfig) (RuleInput, error) {
	in.Nama = strings.TrimSpace(in.Nama)
	if in.Nama == "" || utf8.RuneCountInString(in.Nama) > 200 {
		return in, invalid("nama wajib diisi, maksimal 200 karakter")
	}
	t := &in.Pemicu
	switch t.Jenis {
	case TriggerItemCreated:
		t.ColumnID, t.Dari, t.Ke = nil, nil, nil
	case TriggerStatusChanged:
		if t.ColumnID != nil {
			if _, err := s.column(t.ColumnID, "pemicu", board.TypeStatus); err != nil {
				return in, err
			}
		}
		for _, l := range []*string{t.Dari, t.Ke} {
			if l != nil && !s.hasLabel(t.ColumnID, *l) {
				return in, invalid("pemicu: label status %q tidak ada", *l)
			}
		}
	case TriggerAssigneeChange:
		t.Dari, t.Ke = nil, nil
		if t.ColumnID != nil {
			if _, err := s.column(t.ColumnID, "pemicu", board.TypePerson); err != nil {
				return in, err
			}
		}
	case TriggerDateArrived:
		t.Dari, t.Ke = nil, nil
		if _, err := s.column(t.ColumnID, "pemicu", board.TypeDate, board.TypeTimeline); err != nil {
			return in, err
		}
	default:
		return in, invalid("pemicu tidak dikenal")
	}

	if len(in.Kondisi) > MaxConditions {
		return in, invalid("paling banyak %d kondisi", MaxConditions)
	}
	if in.Kondisi == nil {
		in.Kondisi = []Condition{}
	}
	for i := range in.Kondisi {
		c := &in.Kondisi[i]
		what := fmt.Sprintf("kondisi %d", i+1)
		switch c.Jenis {
		case CondColumnEquals:
			col, err := s.column(c.ColumnID, what, board.TypeStatus, board.TypePriority, board.TypeText,
				board.TypeNumber, board.TypeProgress, board.TypeDate)
			if err != nil {
				return in, err
			}
			if c.Nilai, err = s.normalize(col, c.Nilai, what); err != nil {
				return in, err
			}
			c.UserID, c.GroupID = nil, nil
		case CondPersonHas:
			if _, err := s.column(c.ColumnID, what, board.TypePerson); err != nil {
				return in, err
			}
			if c.UserID == nil || !s.members[*c.UserID] {
				return in, invalid("%s: pengguna harus anggota proyek", what)
			}
			c.Nilai, c.GroupID = nil, nil
		case CondInGroup:
			if c.GroupID == nil || !s.groups[*c.GroupID] {
				return in, invalid("%s: grup tidak ada di board ini", what)
			}
			c.ColumnID, c.Nilai, c.UserID = nil, nil, nil
		default:
			return in, invalid("%s: jenis tidak dikenal", what)
		}
	}

	if len(in.Aksi) == 0 || len(in.Aksi) > MaxActions {
		return in, invalid("aturan harus memiliki 1 sampai %d aksi", MaxActions)
	}
	for i := range in.Aksi {
		a := &in.Aksi[i]
		what := fmt.Sprintf("aksi %d", i+1)
		switch a.Jenis {
		case ActionMoveGroup:
			if a.GroupID == nil || !s.groups[*a.GroupID] {
				return in, invalid("%s: grup tujuan tidak ada di board ini", what)
			}
			a.ColumnID, a.Nilai, a.URL = nil, nil, ""
		case ActionSetValue:
			col, err := s.column(a.ColumnID, what, valueTypes...)
			if err != nil {
				return in, err
			}
			if a.Nilai, err = s.normalize(col, a.Nilai, what); err != nil {
				return in, err
			}
			a.GroupID, a.URL = nil, ""
		case ActionWebhook:
			u, err := webhook.ValidateURL(a.URL)
			if err != nil {
				return in, invalid("%s: %v", what, err)
			}
			a.URL, a.GroupID, a.ColumnID, a.Nilai = u, nil, nil, nil
		default:
			return in, invalid("%s: jenis tidak dikenal", what)
		}
	}
	return in, nil
}

func marshalRule(in RuleInput) (pemicu, kondisi, aksi []byte, err error) {
	if pemicu, err = json.Marshal(in.Pemicu); err != nil {
		return
	}
	if kondisi, err = json.Marshal(in.Kondisi); err != nil {
		return
	}
	aksi, err = json.Marshal(in.Aksi)
	return
}

func record(ctx context.Context, q *dbgen.Queries, actor auth.Principal, meta auth.RequestMeta, id uuid.UUID, aksi string, sebelum, sesudah any) error {
	return audit.Record(ctx, q, audit.Entry{
		AktorID: &actor.User.ID, Entitas: "automation_rule", EntitasID: id.String(), Aksi: aksi,
		Sebelum: sebelum, Sesudah: sesudah, IP: meta.IP,
	})
}

// RuleList adalah respons daftar aturan board.
type RuleList struct {
	Items      []Rule `json:"items"`
	BisaKelola bool   `json:"bisa_kelola"`
}

// List mengembalikan aturan board. Hak baca board; bisa_kelola untuk manager.
func (s *Service) List(ctx context.Context, actor auth.Principal, boardID uuid.UUID) (RuleList, error) {
	if err := board.BoardForRead(ctx, s.q, actor, boardID); err != nil {
		return RuleList{}, err
	}
	rows, err := s.q.ListAutomationRules(ctx, boardID)
	if err != nil {
		return RuleList{}, fmt.Errorf("daftar aturan: %w", err)
	}
	out := RuleList{Items: make([]Rule, 0, len(rows))}
	for _, r := range rows {
		rule, err := toRule(r.AutomationRule, r.DibuatOlehNama)
		if err != nil {
			return RuleList{}, err
		}
		out.Items = append(out.Items, rule)
	}
	_, err = board.BoardForManage(ctx, s.q, actor, boardID)
	out.BisaKelola = err == nil
	return out, nil
}

// Create membuat aturan. Hanya manager efektif proyek pada proyek aktif.
func (s *Service) Create(ctx context.Context, actor auth.Principal, boardID uuid.UUID, in RuleInput, meta auth.RequestMeta) (Rule, error) {
	var out Rule
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		projectID, err := board.BoardForManage(ctx, q, actor, boardID)
		if err != nil {
			return err
		}
		n, err := q.CountAutomationRules(ctx, boardID)
		if err != nil {
			return fmt.Errorf("hitung aturan: %w", err)
		}
		if n >= MaxRulesPerBoard {
			return ErrTooManyRules
		}
		schema, err := loadSchema(ctx, q, boardID, projectID)
		if err != nil {
			return err
		}
		if in, err = schema.validate(in, s.webhook); err != nil {
			return err
		}
		p, k, a, err := marshalRule(in)
		if err != nil {
			return fmt.Errorf("susun aturan: %w", err)
		}
		r, err := q.CreateAutomationRule(ctx, dbgen.CreateAutomationRuleParams{
			BoardID: boardID, Nama: in.Nama, PemicuJson: p, KondisiJson: k, AksiJson: a, Aktif: in.Aktif, DibuatOleh: actor.User.ID,
		})
		if err != nil {
			return fmt.Errorf("buat aturan: %w", err)
		}
		if out, err = toRule(r, actor.User.Nama); err != nil {
			return err
		}
		if out.WebhookRahasia, err = syncSecret(ctx, q, r.ID, in.Aksi); err != nil {
			return err
		}
		return record(ctx, q, actor, meta, r.ID, "buat", nil, in)
	})
	return out, err
}

// lockRule mengunci aturan dan memastikan pelaku manager pada board-nya.
// Aturan pada board yang tidak dapat dibaca dilaporkan tidak ditemukan.
func lockRule(ctx context.Context, q *dbgen.Queries, actor auth.Principal, id uuid.UUID) (dbgen.AutomationRule, uuid.UUID, error) {
	r, err := q.GetAutomationRuleForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, uuid.Nil, ErrNotFound
	}
	if err != nil {
		return r, uuid.Nil, fmt.Errorf("kunci aturan: %w", err)
	}
	if err := board.BoardForRead(ctx, q, actor, r.BoardID); err != nil {
		if errors.Is(err, board.ErrNotFound) || errors.Is(err, project.ErrNotFound) {
			return r, uuid.Nil, ErrNotFound
		}
		return r, uuid.Nil, err
	}
	projectID, err := board.BoardForManage(ctx, q, actor, r.BoardID)
	return r, projectID, err
}

// Update mengganti isi aturan (nama, aktif, pemicu, kondisi, aksi).
func (s *Service) Update(ctx context.Context, actor auth.Principal, id uuid.UUID, in RuleInput, meta auth.RequestMeta) (Rule, error) {
	var out Rule
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		before, projectID, err := lockRule(ctx, q, actor, id)
		if err != nil {
			return err
		}
		schema, err := loadSchema(ctx, q, before.BoardID, projectID)
		if err != nil {
			return err
		}
		if in, err = schema.validate(in, s.webhook); err != nil {
			return err
		}
		p, k, a, err := marshalRule(in)
		if err != nil {
			return fmt.Errorf("susun aturan: %w", err)
		}
		r, err := q.UpdateAutomationRule(ctx, dbgen.UpdateAutomationRuleParams{
			ID: id, Nama: in.Nama, PemicuJson: p, KondisiJson: k, AksiJson: a, Aktif: in.Aktif,
		})
		if err != nil {
			return fmt.Errorf("ubah aturan: %w", err)
		}
		prev, err := toRule(before, "")
		if err != nil {
			return err
		}
		if out, err = toRule(r, ""); err != nil {
			return err
		}
		if out.WebhookRahasia, err = syncSecret(ctx, q, r.ID, in.Aksi); err != nil {
			return err
		}
		return record(ctx, q, actor, meta, id, "ubah", prev, in)
	})
	return out, err
}

// Delete menghapus aturan beserta riwayatnya.
func (s *Service) Delete(ctx context.Context, actor auth.Principal, id uuid.UUID, meta auth.RequestMeta) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		before, _, err := lockRule(ctx, q, actor, id)
		if err != nil {
			return err
		}
		if err := q.DeleteAutomationRule(ctx, id); err != nil {
			return fmt.Errorf("hapus aturan: %w", err)
		}
		prev, err := toRule(before, "")
		if err != nil {
			return err
		}
		return record(ctx, q, actor, meta, id, "hapus", prev, nil)
	})
}

// Run adalah satu eksekusi aturan.
type Run struct {
	ID        int64      `json:"id"`
	ItemID    *uuid.UUID `json:"item_id"`
	ItemJudul *string    `json:"item_judul"`
	Hasil     string     `json:"hasil"`
	Pesan     string     `json:"pesan"`
	Waktu     time.Time  `json:"waktu"`
}

// Runs mengembalikan eksekusi terbaru aturan. Hak baca board.
func (s *Service) Runs(ctx context.Context, actor auth.Principal, id uuid.UUID) ([]Run, error) {
	r, err := s.q.GetAutomationRule(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("baca aturan: %w", err)
	}
	if err := board.BoardForRead(ctx, s.q, actor, r.BoardID); err != nil {
		if errors.Is(err, board.ErrNotFound) || errors.Is(err, project.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rows, err := s.q.ListAutomationRuns(ctx, dbgen.ListAutomationRunsParams{RuleID: id, Limit: MaxRunsListed})
	if err != nil {
		return nil, fmt.Errorf("daftar eksekusi: %w", err)
	}
	out := make([]Run, len(rows))
	for i, x := range rows {
		out[i] = Run{ID: x.ID, ItemID: x.ItemID, ItemJudul: x.ItemJudul, Hasil: x.Hasil, Pesan: x.Pesan, Waktu: x.Waktu}
	}
	return out, nil
}

func hasWebhook(acts []Action) bool {
	return slices.ContainsFunc(acts, func(a Action) bool { return a.Jenis == ActionWebhook })
}

// syncSecret membuat rahasia HMAC bila aturan kini punya aksi webhook dan belum
// punya rahasia (dikembalikan sekali), atau menghapusnya bila tidak lagi dipakai.
func syncSecret(ctx context.Context, q *dbgen.Queries, ruleID uuid.UUID, acts []Action) (*string, error) {
	if !hasWebhook(acts) {
		if err := q.DeleteAutomationSecret(ctx, ruleID); err != nil {
			return nil, fmt.Errorf("hapus rahasia webhook: %w", err)
		}
		return nil, nil
	}
	_, err := q.GetAutomationSecret(ctx, ruleID)
	if err == nil {
		return nil, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("baca rahasia webhook: %w", err)
	}
	return newSecret(ctx, q, ruleID)
}

func newSecret(ctx context.Context, q *dbgen.Queries, ruleID uuid.UUID) (*string, error) {
	secret, err := NewWebhookSecret()
	if err != nil {
		return nil, err
	}
	if err := q.UpsertAutomationSecret(ctx, dbgen.UpsertAutomationSecretParams{RuleID: ruleID, Rahasia: secret}); err != nil {
		return nil, fmt.Errorf("simpan rahasia webhook: %w", err)
	}
	return &secret, nil
}

// RotateSecret mengganti rahasia HMAC webhook aturan; rahasia baru dikembalikan
// sekali. Kiriman yang masih mengantre ditandatangani dengan rahasia baru.
func (s *Service) RotateSecret(ctx context.Context, actor auth.Principal, id uuid.UUID, meta auth.RequestMeta) (string, error) {
	var secret string
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		r, _, err := lockRule(ctx, q, actor, id)
		if err != nil {
			return err
		}
		rule, err := toRule(r, "")
		if err != nil {
			return err
		}
		if !hasWebhook(rule.Aksi) {
			return invalid("aturan ini tidak memiliki aksi webhook")
		}
		sp, err := newSecret(ctx, q, id)
		if err != nil {
			return err
		}
		secret = *sp
		// Nilai rahasia tidak pernah dicatat di audit.
		return record(ctx, q, actor, meta, id, "ganti_rahasia_webhook", nil, nil)
	})
	return secret, err
}

// Delivery adalah satu kiriman webhook.
type Delivery struct {
	ID          int64      `json:"id"`
	URL         string     `json:"url"`
	Status      string     `json:"status"`
	Percobaan   int32      `json:"percobaan"`
	Galat       string     `json:"galat"`
	KodeHTTP    *int32     `json:"kode_http"`
	DibuatPada  time.Time  `json:"dibuat_pada"`
	SelesaiPada *time.Time `json:"selesai_pada"`
}

// Deliveries mengembalikan kiriman webhook terbaru aturan. Hak baca board.
func (s *Service) Deliveries(ctx context.Context, actor auth.Principal, id uuid.UUID) ([]Delivery, error) {
	r, err := s.q.GetAutomationRule(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("baca aturan: %w", err)
	}
	if err := board.BoardForRead(ctx, s.q, actor, r.BoardID); err != nil {
		if errors.Is(err, board.ErrNotFound) || errors.Is(err, project.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rows, err := s.q.ListWebhookDeliveries(ctx, dbgen.ListWebhookDeliveriesParams{RuleID: &id, Limit: MaxRunsListed})
	if err != nil {
		return nil, fmt.Errorf("daftar kiriman webhook: %w", err)
	}
	out := make([]Delivery, len(rows))
	for i, x := range rows {
		out[i] = Delivery{
			ID: x.ID, URL: x.Url, Status: x.Status, Percobaan: x.Percobaan, Galat: x.Galat,
			KodeHTTP: x.KodeHttp, DibuatPada: x.DibuatPada, SelesaiPada: x.SelesaiPada,
		}
	}
	return out, nil
}

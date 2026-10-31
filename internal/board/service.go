package board

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"jwm/internal/audit"
	"jwm/internal/auth"
	"jwm/internal/db"
	"jwm/internal/db/gen"
	"jwm/internal/httpx"
	"jwm/internal/project"
)

// ProjectRole adalah hak efektif pada proyek pemilik board.
type ProjectRole = project.Role

// Alias peran proyek agar aturan di paket ini mudah dibaca.
const (
	RoleNone    = project.RoleNone
	RoleViewer  = project.RoleViewer
	RoleMember  = project.RoleMember
	RoleManager = project.RoleManager
)

// Galat layanan board. Galat akses (tidak ditemukan, tidak berhak, arsip)
// memakai galat paket project.
var (
	ErrNotFound      = errors.New("board, kolom, grup, atau item tidak ditemukan")
	ErrInvalidName   = errors.New("nama atau judul wajib diisi dan tidak melebihi batas panjang")
	ErrInvalidOrder  = errors.New("urutan harus angka berhingga")
	ErrGroupNotEmpty = errors.New("grup masih berisi item; pindahkan atau hapus item terlebih dahulu")
	ErrLastGroup     = errors.New("board harus memiliki minimal satu grup")
	ErrLabelInUse    = errors.New("label status masih dipakai item; ubah nilai item terlebih dahulu")
	ErrWrongBoard    = errors.New("grup, kolom, atau item acuan tidak berada di board yang sama")
)

// Service adalah logika bisnis board, kolom, grup, dan item.
type Service struct {
	pool    *pgxpool.Pool
	q       *dbgen.Queries
	cascade Cascader
}

// NewService membuat layanan board.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: dbgen.New(pool)}
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func validText(s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > max {
		return "", ErrInvalidName
	}
	return s, nil
}

func validOrder(f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return ErrInvalidOrder
	}
	return nil
}

// boardAccess membaca board aktif dan memeriksa hak pelaku pada proyeknya.
func boardAccess(ctx context.Context, q *dbgen.Queries, actor auth.Principal, boardID uuid.UUID, need ProjectRole, write bool) (dbgen.Board, ProjectRole, error) {
	b, err := q.GetBoard(ctx, boardID)
	if err != nil {
		return b, RoleNone, fmt.Errorf("baca board: %w", notFound(err))
	}
	eff, err := project.Access(ctx, q, actor, b.ProjectID, need, write)
	if errors.Is(err, project.ErrNotFound) {
		return b, eff, ErrNotFound
	}
	return b, eff, err
}

// projectAccess memeriksa hak pada proyek dan menyamarkan "proyek tidak ada" menjadi ErrNotFound board.
func projectAccess(ctx context.Context, q *dbgen.Queries, actor auth.Principal, projectID uuid.UUID, need ProjectRole, write bool) (ProjectRole, error) {
	eff, err := project.Access(ctx, q, actor, projectID, need, write)
	if errors.Is(err, project.ErrNotFound) {
		return eff, ErrNotFound
	}
	return eff, err
}

func record(ctx context.Context, q *dbgen.Queries, actor auth.Principal, meta auth.RequestMeta, entitas string, id uuid.UUID, aksi string, sebelum, sesudah any) error {
	return audit.Record(ctx, q, audit.Entry{
		AktorID: &actor.User.ID, Entitas: entitas, EntitasID: id.String(), Aksi: aksi,
		Sebelum: sebelum, Sesudah: sesudah, IP: meta.IP,
	})
}

// Board adalah tampilan board.
type Board struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	Nama      string    `json:"nama"`
	Urutan    float64   `json:"urutan"`
}

func toBoard(b dbgen.Board) Board {
	return Board{ID: b.ID, ProjectID: b.ProjectID, Nama: b.Nama, Urutan: b.Urutan}
}

// Column adalah tampilan kolom.
type Column struct {
	ID          uuid.UUID       `json:"id"`
	BoardID     uuid.UUID       `json:"board_id"`
	Tipe        ColumnType      `json:"tipe"`
	Judul       string          `json:"judul"`
	Urutan      float64         `json:"urutan"`
	Konfigurasi json.RawMessage `json:"konfigurasi"`
}

func toColumn(c dbgen.BoardColumn) Column {
	return Column{
		ID: c.ID, BoardID: c.BoardID, Tipe: ColumnType(c.Tipe), Judul: c.Judul,
		Urutan: c.Urutan, Konfigurasi: c.KonfigurasiJson,
	}
}

// Group adalah tampilan grup.
type Group struct {
	ID      uuid.UUID `json:"id"`
	BoardID uuid.UUID `json:"board_id"`
	Judul   string    `json:"judul"`
	Warna   string    `json:"warna"`
	Urutan  float64   `json:"urutan"`
}

func toGroup(g dbgen.Group) Group {
	return Group{ID: g.ID, BoardID: g.BoardID, Judul: g.Judul, Warna: g.Warna, Urutan: g.Urutan}
}

// BoardDetail adalah board lengkap dengan kolom dan grupnya.
type BoardDetail struct {
	Board
	Kolom     []Column    `json:"kolom"`
	Grup      []Group     `json:"grup"`
	PeranSaya ProjectRole `json:"peran_saya"`
}

// ListBoards mengembalikan board aktif pada proyek.
func (s *Service) ListBoards(ctx context.Context, actor auth.Principal, projectID uuid.UUID) ([]Board, error) {
	if _, err := projectAccess(ctx, s.q, actor, projectID, RoleViewer, false); err != nil {
		return nil, err
	}
	rows, err := s.q.ListBoards(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("daftar board: %w", err)
	}
	out := make([]Board, len(rows))
	for i, b := range rows {
		out[i] = toBoard(b)
	}
	return out, nil
}

// GetBoard mengembalikan board beserta kolom dan grup.
func (s *Service) GetBoard(ctx context.Context, actor auth.Principal, id uuid.UUID) (BoardDetail, error) {
	b, eff, err := boardAccess(ctx, s.q, actor, id, RoleViewer, false)
	if err != nil {
		return BoardDetail{}, err
	}
	cols, err := s.q.ListColumns(ctx, id)
	if err != nil {
		return BoardDetail{}, fmt.Errorf("daftar kolom: %w", err)
	}
	groups, err := s.q.ListGroups(ctx, id)
	if err != nil {
		return BoardDetail{}, fmt.Errorf("daftar grup: %w", err)
	}
	d := BoardDetail{Board: toBoard(b), Kolom: make([]Column, len(cols)), Grup: make([]Group, len(groups)), PeranSaya: eff}
	for i, c := range cols {
		d.Kolom[i] = toColumn(c)
	}
	for i, g := range groups {
		d.Grup[i] = toGroup(g)
	}
	return d, nil
}

// CreateBoard membuat board. Hanya manager efektif.
// ASSUMPTION: board baru berisi satu grup "Tugas", kolom "Status" berlabel
// bawaan SPEC M2, dan kolom person "Penanggung jawab" (dipakai aturan risiko M8).
func (s *Service) CreateBoard(ctx context.Context, actor auth.Principal, projectID uuid.UUID, nama string, meta auth.RequestMeta) (BoardDetail, error) {
	nama, err := validText(nama, 200)
	if err != nil {
		return BoardDetail{}, err
	}
	var id uuid.UUID
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := q.GetProjectForUpdate(ctx, projectID); err != nil {
			return fmt.Errorf("kunci proyek: %w", notFound(err))
		}
		if _, err := projectAccess(ctx, q, actor, projectID, RoleManager, true); err != nil {
			return err
		}
		urutan, err := q.NextBoardUrutan(ctx, projectID)
		if err != nil {
			return fmt.Errorf("urutan board: %w", err)
		}
		b, err := q.CreateBoard(ctx, dbgen.CreateBoardParams{ProjectID: projectID, Nama: nama, Urutan: urutan})
		if err != nil {
			return fmt.Errorf("buat board: %w", err)
		}
		id = b.ID
		if _, err := q.CreateGroup(ctx, dbgen.CreateGroupParams{BoardID: b.ID, Judul: "Tugas", Warna: "biru", Urutan: positionStep}); err != nil {
			return fmt.Errorf("buat grup bawaan: %w", err)
		}
		statusCfg, err := NormalizeConfig(TypeStatus, nil)
		if err != nil {
			return err
		}
		defaults := []dbgen.CreateColumnParams{
			{BoardID: b.ID, Tipe: string(TypeStatus), Judul: "Status", Urutan: positionStep, KonfigurasiJson: statusCfg},
			{BoardID: b.ID, Tipe: string(TypePerson), Judul: "Penanggung jawab", Urutan: 2 * positionStep, KonfigurasiJson: []byte(`{}`)},
		}
		for _, c := range defaults {
			if _, err := q.CreateColumn(ctx, c); err != nil {
				return fmt.Errorf("buat kolom bawaan: %w", err)
			}
		}
		return record(ctx, q, actor, meta, "board", b.ID, "buat", nil, map[string]any{"project_id": projectID, "nama": nama})
	})
	if err != nil {
		return BoardDetail{}, err
	}
	return s.GetBoard(ctx, actor, id)
}

// lockBoard mengunci board aktif dan memeriksa hak manager pada proyek yang tidak diarsipkan.
func lockBoard(ctx context.Context, q *dbgen.Queries, actor auth.Principal, id uuid.UUID) (dbgen.Board, error) {
	b, err := q.GetBoardForUpdate(ctx, id)
	if err != nil {
		return b, fmt.Errorf("kunci board: %w", notFound(err))
	}
	if _, err := projectAccess(ctx, q, actor, b.ProjectID, RoleManager, true); err != nil {
		return b, err
	}
	return b, nil
}

// UpdateBoardInput berisi perubahan board.
type UpdateBoardInput struct {
	Nama   httpx.Optional[string]
	Urutan httpx.Optional[float64]
}

// UpdateBoard mengubah nama atau urutan board. Hanya manager efektif.
func (s *Service) UpdateBoard(ctx context.Context, actor auth.Principal, id uuid.UUID, in UpdateBoardInput, meta auth.RequestMeta) (BoardDetail, error) {
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		before, err := lockBoard(ctx, q, actor, id)
		if err != nil {
			return err
		}
		p := dbgen.UpdateBoardParams{ID: id, Nama: before.Nama, Urutan: before.Urutan}
		if in.Nama.Set {
			if in.Nama.Null {
				return ErrInvalidName
			}
			if p.Nama, err = validText(in.Nama.Value, 200); err != nil {
				return err
			}
		}
		if in.Urutan.Set {
			if in.Urutan.Null {
				return ErrInvalidOrder
			}
			if err := validOrder(in.Urutan.Value); err != nil {
				return err
			}
			p.Urutan = in.Urutan.Value
		}
		after, err := q.UpdateBoard(ctx, p)
		if err != nil {
			return fmt.Errorf("ubah board: %w", err)
		}
		return record(ctx, q, actor, meta, "board", id, "ubah", toBoard(before), toBoard(after))
	})
	if err != nil {
		return BoardDetail{}, err
	}
	return s.GetBoard(ctx, actor, id)
}

// DeleteBoard menghapus lunak board. Hanya manager efektif.
func (s *Service) DeleteBoard(ctx context.Context, actor auth.Principal, id uuid.UUID, meta auth.RequestMeta) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		b, err := lockBoard(ctx, q, actor, id)
		if err != nil {
			return err
		}
		if err := q.SoftDeleteBoard(ctx, id); err != nil {
			return fmt.Errorf("hapus board: %w", err)
		}
		return record(ctx, q, actor, meta, "board", id, "hapus", toBoard(b), nil)
	})
}

// CreateColumn menambah kolom bertipe di akhir board. Hanya manager efektif.
func (s *Service) CreateColumn(ctx context.Context, actor auth.Principal, boardID uuid.UUID, tipe ColumnType, judul string, konfigurasi json.RawMessage, meta auth.RequestMeta) (Column, error) {
	judul, err := validText(judul, 100)
	if err != nil {
		return Column{}, err
	}
	cfg, err := NormalizeConfig(tipe, konfigurasi)
	if err != nil {
		return Column{}, err
	}
	var out Column
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := lockBoard(ctx, q, actor, boardID); err != nil {
			return err
		}
		urutan, err := q.NextColumnUrutan(ctx, boardID)
		if err != nil {
			return fmt.Errorf("urutan kolom: %w", err)
		}
		c, err := q.CreateColumn(ctx, dbgen.CreateColumnParams{
			BoardID: boardID, Tipe: string(tipe), Judul: judul, Urutan: urutan, KonfigurasiJson: cfg,
		})
		if err != nil {
			return fmt.Errorf("buat kolom: %w", err)
		}
		out = toColumn(c)
		return record(ctx, q, actor, meta, "board_column", c.ID, "buat", nil, out)
	})
	return out, err
}

// columnForWrite mengunci kolom aktif dan memeriksa hak manager.
func columnForWrite(ctx context.Context, q *dbgen.Queries, actor auth.Principal, id uuid.UUID) (dbgen.BoardColumn, error) {
	row, err := q.GetColumnWithProject(ctx, id)
	if err != nil {
		return dbgen.BoardColumn{}, fmt.Errorf("baca kolom: %w", notFound(err))
	}
	if _, err := lockBoard(ctx, q, actor, row.BoardColumn.BoardID); err != nil {
		return dbgen.BoardColumn{}, err
	}
	c, err := q.GetColumnForUpdate(ctx, id)
	if err != nil {
		return c, fmt.Errorf("kunci kolom: %w", notFound(err))
	}
	return c, nil
}

// UpdateColumnInput berisi perubahan kolom. Tipe kolom tidak dapat diubah.
type UpdateColumnInput struct {
	Judul       httpx.Optional[string]
	Urutan      httpx.Optional[float64]
	Konfigurasi httpx.Optional[json.RawMessage]
}

// UpdateColumn mengubah judul, urutan, atau konfigurasi kolom. Hanya manager efektif.
func (s *Service) UpdateColumn(ctx context.Context, actor auth.Principal, id uuid.UUID, in UpdateColumnInput, meta auth.RequestMeta) (Column, error) {
	var out Column
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		before, err := columnForWrite(ctx, q, actor, id)
		if err != nil {
			return err
		}
		p := dbgen.UpdateColumnParams{ID: id, Judul: before.Judul, Urutan: before.Urutan, KonfigurasiJson: before.KonfigurasiJson}
		if in.Judul.Set {
			if in.Judul.Null {
				return ErrInvalidName
			}
			if p.Judul, err = validText(in.Judul.Value, 100); err != nil {
				return err
			}
		}
		if in.Urutan.Set {
			if in.Urutan.Null {
				return ErrInvalidOrder
			}
			if err := validOrder(in.Urutan.Value); err != nil {
				return err
			}
			p.Urutan = in.Urutan.Value
		}
		if in.Konfigurasi.Set {
			tipe := ColumnType(before.Tipe)
			if p.KonfigurasiJson, err = NormalizeConfig(tipe, in.Konfigurasi.Value); err != nil {
				return err
			}
			if tipe == TypeStatus {
				ids, err := statusLabelIDs(p.KonfigurasiJson)
				if err != nil {
					return err
				}
				n, err := q.CountStatusValuesOutside(ctx, dbgen.CountStatusValuesOutsideParams{ColumnID: id, Labels: ids})
				if err != nil {
					return fmt.Errorf("periksa label: %w", err)
				}
				if n > 0 {
					return ErrLabelInUse
				}
			}
		}
		after, err := q.UpdateColumn(ctx, p)
		if err != nil {
			return fmt.Errorf("ubah kolom: %w", err)
		}
		out = toColumn(after)
		return record(ctx, q, actor, meta, "board_column", id, "ubah", toColumn(before), out)
	})
	return out, err
}

// DeleteColumn menghapus lunak kolom; nilai sel tetap tersimpan. Hanya manager efektif.
func (s *Service) DeleteColumn(ctx context.Context, actor auth.Principal, id uuid.UUID, meta auth.RequestMeta) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		c, err := columnForWrite(ctx, q, actor, id)
		if err != nil {
			return err
		}
		if err := q.SoftDeleteColumn(ctx, id); err != nil {
			return fmt.Errorf("hapus kolom: %w", err)
		}
		return record(ctx, q, actor, meta, "board_column", id, "hapus", toColumn(c), nil)
	})
}

// ASSUMPTION: grup dikelola manager (bagian dari struktur board), warna default biru.
const defaultGroupColor = "biru"

// CreateGroup menambah grup di akhir board. Hanya manager efektif.
func (s *Service) CreateGroup(ctx context.Context, actor auth.Principal, boardID uuid.UUID, judul, warna string, meta auth.RequestMeta) (Group, error) {
	judul, err := validText(judul, 100)
	if err != nil {
		return Group{}, err
	}
	if warna == "" {
		warna = defaultGroupColor
	}
	if !ValidColor(warna) {
		return Group{}, ErrInvalidColor
	}
	var out Group
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := lockBoard(ctx, q, actor, boardID); err != nil {
			return err
		}
		urutan, err := q.NextGroupUrutan(ctx, boardID)
		if err != nil {
			return fmt.Errorf("urutan grup: %w", err)
		}
		g, err := q.CreateGroup(ctx, dbgen.CreateGroupParams{BoardID: boardID, Judul: judul, Warna: warna, Urutan: urutan})
		if err != nil {
			return fmt.Errorf("buat grup: %w", err)
		}
		out = toGroup(g)
		return record(ctx, q, actor, meta, "group", g.ID, "buat", nil, out)
	})
	return out, err
}

// groupForWrite mengunci board dan grup aktif, lalu memeriksa hak manager.
func groupForWrite(ctx context.Context, q *dbgen.Queries, actor auth.Principal, id uuid.UUID) (dbgen.Group, error) {
	row, err := q.GetGroupWithProject(ctx, id)
	if err != nil {
		return dbgen.Group{}, fmt.Errorf("baca grup: %w", notFound(err))
	}
	if _, err := lockBoard(ctx, q, actor, row.Group.BoardID); err != nil {
		return dbgen.Group{}, err
	}
	g, err := q.GetGroupForUpdate(ctx, id)
	if err != nil {
		return g, fmt.Errorf("kunci grup: %w", notFound(err))
	}
	return g, nil
}

// UpdateGroupInput berisi perubahan grup.
type UpdateGroupInput struct {
	Judul  httpx.Optional[string]
	Warna  httpx.Optional[string]
	Urutan httpx.Optional[float64]
}

// UpdateGroup mengubah judul, warna, atau urutan grup. Hanya manager efektif.
func (s *Service) UpdateGroup(ctx context.Context, actor auth.Principal, id uuid.UUID, in UpdateGroupInput, meta auth.RequestMeta) (Group, error) {
	var out Group
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		before, err := groupForWrite(ctx, q, actor, id)
		if err != nil {
			return err
		}
		p := dbgen.UpdateGroupParams{ID: id, Judul: before.Judul, Warna: before.Warna, Urutan: before.Urutan}
		if in.Judul.Set {
			if in.Judul.Null {
				return ErrInvalidName
			}
			if p.Judul, err = validText(in.Judul.Value, 100); err != nil {
				return err
			}
		}
		if in.Warna.Set {
			if in.Warna.Null || !ValidColor(in.Warna.Value) {
				return ErrInvalidColor
			}
			p.Warna = in.Warna.Value
		}
		if in.Urutan.Set {
			if in.Urutan.Null {
				return ErrInvalidOrder
			}
			if err := validOrder(in.Urutan.Value); err != nil {
				return err
			}
			p.Urutan = in.Urutan.Value
		}
		after, err := q.UpdateGroup(ctx, p)
		if err != nil {
			return fmt.Errorf("ubah grup: %w", err)
		}
		out = toGroup(after)
		return record(ctx, q, actor, meta, "group", id, "ubah", toGroup(before), out)
	})
	return out, err
}

// DeleteGroup menghapus lunak grup kosong. Grup terakhir tidak dapat dihapus.
// Hanya manager efektif.
func (s *Service) DeleteGroup(ctx context.Context, actor auth.Principal, id uuid.UUID, meta auth.RequestMeta) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		g, err := groupForWrite(ctx, q, actor, id)
		if err != nil {
			return err
		}
		n, err := q.CountActiveItemsInGroup(ctx, id)
		if err != nil {
			return fmt.Errorf("hitung item: %w", err)
		}
		if n > 0 {
			return ErrGroupNotEmpty
		}
		groups, err := q.CountActiveGroups(ctx, g.BoardID)
		if err != nil {
			return fmt.Errorf("hitung grup: %w", err)
		}
		if groups <= 1 {
			return ErrLastGroup
		}
		if err := q.SoftDeleteGroup(ctx, id); err != nil {
			return fmt.Errorf("hapus grup: %w", err)
		}
		return record(ctx, q, actor, meta, "group", id, "hapus", toGroup(g), nil)
	})
}

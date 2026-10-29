package board

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"jwm/internal/auth"
	"jwm/internal/db"
	"jwm/internal/db/gen"
	"jwm/internal/project"
)

// Item adalah tampilan item beserta nilai selnya.
type Item struct {
	ID         uuid.UUID                  `json:"id"`
	BoardID    uuid.UUID                  `json:"board_id"`
	GroupID    uuid.UUID                  `json:"group_id"`
	Judul      string                     `json:"judul"`
	Urutan     float64                    `json:"urutan"`
	DibuatOleh uuid.UUID                  `json:"dibuat_oleh"`
	DibuatPada time.Time                  `json:"dibuat_pada"`
	DiubahPada time.Time                  `json:"diubah_pada"`
	Nilai      map[string]json.RawMessage `json:"nilai"`
	// BisaUbah dan BisaHapus adalah hak pengguna yang meminta atas item ini.
	BisaUbah  bool `json:"bisa_ubah"`
	BisaHapus bool `json:"bisa_hapus"`
}

// ItemPage adalah satu halaman item, urut id (waktu pembuatan).
// Klien mengurutkan tampilan menurut urutan grup dan urutan item.
type ItemPage struct {
	Items         []Item     `json:"items"`
	KursorBerikut *uuid.UUID `json:"kursor_berikut"`
}

// ItemFilter menyaring daftar item.
type ItemFilter struct {
	GroupID *uuid.UUID
	Setelah *uuid.UUID
	Batas   int
}

// Batas halaman item.
const (
	DefaultItemLimit = 200
	MaxItemLimit     = 1000
)

func (s *Service) toItems(rows []dbgen.ListItemsRow, eff ProjectRole, actor uuid.UUID) ([]Item, error) {
	out := make([]Item, len(rows))
	for i, r := range rows {
		nilai := map[string]json.RawMessage{}
		if err := json.Unmarshal(r.Nilai, &nilai); err != nil {
			return nil, fmt.Errorf("baca nilai item: %w", err)
		}
		out[i] = Item{
			ID: r.ID, BoardID: r.BoardID, GroupID: r.GroupID, Judul: r.Judul, Urutan: r.Urutan,
			DibuatOleh: r.DibuatOleh, DibuatPada: r.DibuatPada, DiubahPada: r.DiubahPada, Nilai: nilai,
			BisaUbah:  canEditItem(eff, r.DibuatOleh, actor, r.Ditugaskan),
			BisaHapus: canDeleteItem(eff, r.DibuatOleh, actor),
		}
	}
	return out, nil
}

// ListItems mengembalikan item aktif pada board dengan paginasi kursor.
func (s *Service) ListItems(ctx context.Context, actor auth.Principal, boardID uuid.UUID, f ItemFilter) (ItemPage, error) {
	_, eff, err := boardAccess(ctx, s.q, actor, boardID, RoleViewer, false)
	if err != nil {
		return ItemPage{}, err
	}
	limit := f.Batas
	if limit <= 0 {
		limit = DefaultItemLimit
	}
	limit = min(limit, MaxItemLimit)
	rows, err := s.q.ListItems(ctx, dbgen.ListItemsParams{
		UserID: actor.User.ID.String(), BoardID: boardID, GroupID: f.GroupID, SetelahID: f.Setelah, Batas: int32(limit),
	})
	if err != nil {
		return ItemPage{}, fmt.Errorf("daftar item: %w", err)
	}
	items, err := s.toItems(rows, eff, actor.User.ID)
	if err != nil {
		return ItemPage{}, err
	}
	page := ItemPage{Items: items}
	if len(rows) == limit {
		last := rows[len(rows)-1].ID
		page.KursorBerikut = &last
	}
	return page, nil
}

// GetItem mengembalikan satu item aktif.
func (s *Service) GetItem(ctx context.Context, actor auth.Principal, id uuid.UUID) (Item, error) {
	row, err := s.q.GetItemAccess(ctx, dbgen.GetItemAccessParams{ID: id, UserID: actor.User.ID.String()})
	if err != nil {
		return Item{}, fmt.Errorf("baca item: %w", notFound(err))
	}
	eff, err := projectAccess(ctx, s.q, actor, row.ProjectID, RoleViewer, false)
	if err != nil {
		return Item{}, err
	}
	rows, err := s.q.ListItems(ctx, dbgen.ListItemsParams{
		UserID: actor.User.ID.String(), BoardID: row.Item.BoardID, ID: &id, Batas: 1,
	})
	if err != nil {
		return Item{}, fmt.Errorf("baca item: %w", err)
	}
	if len(rows) == 0 {
		return Item{}, ErrNotFound
	}
	items, err := s.toItems(rows, eff, actor.User.ID)
	if err != nil {
		return Item{}, err
	}
	return items[0], nil
}

// lockGroupShared mengunci grup aktif (FOR SHARE) agar tidak dihapus selama transaksi.
func lockGroupShared(ctx context.Context, q *dbgen.Queries, groupID, boardID uuid.UUID) (dbgen.Group, error) {
	g, err := q.GetGroupForShare(ctx, groupID)
	if errors.Is(err, pgx.ErrNoRows) {
		return g, ErrWrongBoard
	}
	if err != nil {
		return g, fmt.Errorf("kunci grup: %w", err)
	}
	if g.BoardID != boardID {
		return g, ErrWrongBoard
	}
	return g, nil
}

// CreateItem membuat item di akhir grup. Member efektif ke atas.
func (s *Service) CreateItem(ctx context.Context, actor auth.Principal, boardID, groupID uuid.UUID, judul string, meta auth.RequestMeta) (Item, error) {
	judul, err := validText(judul, 500)
	if err != nil {
		return Item{}, err
	}
	var id uuid.UUID
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, _, err := boardAccess(ctx, q, actor, boardID, RoleMember, true); err != nil {
			return err
		}
		if _, err := lockGroupShared(ctx, q, groupID, boardID); err != nil {
			return err
		}
		last, err := q.MaxItemUrutan(ctx, groupID)
		if err != nil {
			return fmt.Errorf("urutan item: %w", err)
		}
		it, err := q.CreateItem(ctx, dbgen.CreateItemParams{
			BoardID: boardID, GroupID: groupID, Judul: judul, Urutan: last + positionStep, DibuatOleh: actor.User.ID,
		})
		if err != nil {
			return fmt.Errorf("buat item: %w", err)
		}
		id = it.ID
		return record(ctx, q, actor, meta, "item", it.ID, "buat", nil,
			map[string]any{"board_id": boardID, "group_id": groupID, "judul": judul})
	})
	if err != nil {
		return Item{}, err
	}
	return s.GetItem(ctx, actor, id)
}

// itemForWrite mengunci item aktif dan memeriksa hak ubah (atau hapus) pelaku.
func itemForWrite(ctx context.Context, q *dbgen.Queries, actor auth.Principal, id uuid.UUID, forDelete bool) (dbgen.Item, uuid.UUID, error) {
	it, err := q.GetItemForUpdate(ctx, id)
	if err != nil {
		return it, uuid.Nil, fmt.Errorf("kunci item: %w", notFound(err))
	}
	row, err := q.GetItemAccess(ctx, dbgen.GetItemAccessParams{ID: id, UserID: actor.User.ID.String()})
	if err != nil {
		return it, uuid.Nil, fmt.Errorf("baca item: %w", notFound(err))
	}
	eff, err := projectAccess(ctx, q, actor, row.ProjectID, RoleViewer, false)
	if err != nil {
		return it, uuid.Nil, err
	}
	allowed := canEditItem(eff, it.DibuatOleh, actor.User.ID, row.Ditugaskan)
	if forDelete {
		allowed = canDeleteItem(eff, it.DibuatOleh, actor.User.ID)
	}
	if !allowed {
		return it, uuid.Nil, project.ErrForbidden
	}
	// Proyek arsip hanya-baca; diperiksa setelah hak agar viewer tetap mendapat 403.
	if _, err := projectAccess(ctx, q, actor, row.ProjectID, RoleViewer, true); err != nil {
		return it, uuid.Nil, err
	}
	return it, row.ProjectID, nil
}

// UpdateItemTitle mengubah judul item.
func (s *Service) UpdateItemTitle(ctx context.Context, actor auth.Principal, id uuid.UUID, judul string, meta auth.RequestMeta) (Item, error) {
	judul, err := validText(judul, 500)
	if err != nil {
		return Item{}, err
	}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		it, _, err := itemForWrite(ctx, q, actor, id, false)
		if err != nil {
			return err
		}
		if err := q.UpdateItemTitle(ctx, dbgen.UpdateItemTitleParams{ID: id, Judul: judul}); err != nil {
			return fmt.Errorf("ubah judul: %w", err)
		}
		return record(ctx, q, actor, meta, "item", id, "ubah_judul",
			map[string]string{"judul": it.Judul}, map[string]string{"judul": judul})
	})
	if err != nil {
		return Item{}, err
	}
	return s.GetItem(ctx, actor, id)
}

// MoveItem memindah item ke grup tujuan, tepat setelah item acuan
// (setelah nil berarti posisi paling atas grup).
func (s *Service) MoveItem(ctx context.Context, actor auth.Principal, id, groupID uuid.UUID, setelah *uuid.UUID, meta auth.RequestMeta) (Item, error) {
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		it, _, err := itemForWrite(ctx, q, actor, id, false)
		if err != nil {
			return err
		}
		if _, err := lockGroupShared(ctx, q, groupID, it.BoardID); err != nil {
			return err
		}
		urutan, err := targetPosition(ctx, q, id, groupID, setelah)
		if err != nil {
			return err
		}
		if err := q.MoveItem(ctx, dbgen.MoveItemParams{ID: id, GroupID: groupID, Urutan: urutan}); err != nil {
			return fmt.Errorf("pindah item: %w", err)
		}
		return record(ctx, q, actor, meta, "item", id, "pindah",
			map[string]any{"group_id": it.GroupID, "urutan": it.Urutan},
			map[string]any{"group_id": groupID, "urutan": urutan})
	})
	if err != nil {
		return Item{}, err
	}
	return s.GetItem(ctx, actor, id)
}

// targetPosition menghitung urutan baru; menata ulang grup bila celahnya habis.
func targetPosition(ctx context.Context, q *dbgen.Queries, id, groupID uuid.UUID, setelah *uuid.UUID) (float64, error) {
	for attempt := 0; attempt < 2; attempt++ {
		var prev, next *float64
		if setelah != nil {
			if *setelah == id {
				return 0, ErrWrongBoard
			}
			ref, err := q.GetItemForUpdate(ctx, *setelah)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && ref.GroupID != groupID) {
				return 0, ErrWrongBoard
			}
			if err != nil {
				return 0, fmt.Errorf("baca item acuan: %w", err)
			}
			prev = &ref.Urutan
			n, err := q.NextItemUrutanAfter(ctx, dbgen.NextItemUrutanAfterParams{GroupID: groupID, Urutan: ref.Urutan, Kecuali: id})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return 0, fmt.Errorf("baca tetangga: %w", err)
			}
			if err == nil {
				next = &n
			}
		} else {
			n, err := q.MinItemUrutan(ctx, dbgen.MinItemUrutanParams{GroupID: groupID, Kecuali: id})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return 0, fmt.Errorf("baca tetangga: %w", err)
			}
			if err == nil {
				next = &n
			}
		}
		if pos, ok := between(prev, next); ok {
			return pos, nil
		}
		if err := q.RenumberGroupItems(ctx, groupID); err != nil {
			return 0, fmt.Errorf("tata ulang urutan: %w", err)
		}
	}
	return 0, errors.New("posisi item tidak dapat dihitung setelah penataan ulang")
}

// SetValue mengisi atau menghapus (nilai null) satu sel item.
func (s *Service) SetValue(ctx context.Context, actor auth.Principal, itemID, columnID uuid.UUID, nilai json.RawMessage, meta auth.RequestMeta) (Item, error) {
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		it, projectID, err := itemForWrite(ctx, q, actor, itemID, false)
		if err != nil {
			return err
		}
		col, err := q.GetColumnWithProject(ctx, columnID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && col.BoardColumn.BoardID != it.BoardID) {
			return ErrWrongBoard
		}
		if err != nil {
			return fmt.Errorf("baca kolom: %w", err)
		}
		tipe := ColumnType(col.BoardColumn.Tipe)

		var before json.RawMessage
		old, err := q.GetItemValue(ctx, dbgen.GetItemValueParams{ItemID: itemID, ColumnID: columnID})
		switch {
		case err == nil:
			before = old.NilaiJson
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("baca nilai: %w", err)
		}

		var after json.RawMessage
		if isEmptyJSON(nilai) {
			if tipe == TypeDependency {
				return ErrReadOnlyColumn
			}
			if err := q.DeleteItemValue(ctx, dbgen.DeleteItemValueParams{ItemID: itemID, ColumnID: columnID}); err != nil {
				return fmt.Errorf("hapus nilai: %w", err)
			}
		} else {
			vc := ValueContext{Config: col.BoardColumn.KonfigurasiJson}
			if tipe == TypePerson {
				members, err := q.ListProjectMemberIDs(ctx, projectID)
				if err != nil {
					return fmt.Errorf("baca anggota: %w", err)
				}
				set := make(map[uuid.UUID]bool, len(members))
				for _, m := range members {
					set[m] = true
				}
				vc.IsMember = func(id uuid.UUID) bool { return set[id] }
			}
			if after, err = NormalizeValue(tipe, nilai, vc); err != nil {
				return err
			}
			if err := q.UpsertItemValue(ctx, dbgen.UpsertItemValueParams{
				ItemID: itemID, ColumnID: columnID, NilaiJson: after, DiubahOleh: &actor.User.ID,
			}); err != nil {
				return fmt.Errorf("simpan nilai: %w", err)
			}
		}
		if err := q.TouchItem(ctx, itemID); err != nil {
			return fmt.Errorf("perbarui item: %w", err)
		}
		return record(ctx, q, actor, meta, "item", itemID, "ubah_nilai",
			map[string]any{"column_id": columnID, "tipe": tipe, "nilai": before},
			map[string]any{"column_id": columnID, "tipe": tipe, "nilai": after})
	})
	if err != nil {
		return Item{}, err
	}
	return s.GetItem(ctx, actor, itemID)
}

// DeleteItem menghapus lunak item.
func (s *Service) DeleteItem(ctx context.Context, actor auth.Principal, id uuid.UUID, meta auth.RequestMeta) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		it, _, err := itemForWrite(ctx, q, actor, id, true)
		if err != nil {
			return err
		}
		if err := q.SoftDeleteItem(ctx, id); err != nil {
			return fmt.Errorf("hapus item: %w", err)
		}
		return record(ctx, q, actor, meta, "item", id, "hapus",
			map[string]any{"judul": it.Judul, "group_id": it.GroupID}, nil)
	})
}

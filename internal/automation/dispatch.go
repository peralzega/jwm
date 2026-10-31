package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"jwm/internal/db/gen"
)

// Status kiriman webhook (webhook_outbox.status).
const (
	DeliveryPending = "menunggu"
	DeliveryOK      = "terkirim"
	DeliveryFailed  = "gagal"
)

const dispatchBatch = 10

// webhookPayload adalah isi kiriman: data minimal tentang kejadian, bukan seluruh item.
type webhookPayload struct {
	Jenis  string    `json:"jenis"`
	Waktu  time.Time `json:"waktu"`
	Aturan struct {
		ID   uuid.UUID `json:"id"`
		Nama string    `json:"nama"`
	} `json:"aturan"`
	Pemicu struct {
		Jenis    string          `json:"jenis"`
		ColumnID *uuid.UUID      `json:"column_id,omitempty"`
		Sebelum  json.RawMessage `json:"sebelum,omitempty"`
		Sesudah  json.RawMessage `json:"sesudah,omitempty"`
	} `json:"pemicu"`
	Item struct {
		ID    uuid.UUID `json:"id"`
		Judul string    `json:"judul"`
		URL   string    `json:"url,omitempty"`
	} `json:"item"`
	Board  ref `json:"board"`
	Proyek ref `json:"proyek"`
}

type ref struct {
	ID   uuid.UUID `json:"id"`
	Nama string    `json:"nama"`
}

// enqueueWebhook memasukkan kiriman ke antrean dalam transaksi aturan, sehingga
// kiriman hanya ada bila seluruh aksi aturan berhasil.
func (w *Worker) enqueueWebhook(ctx context.Context, q *dbgen.Queries, e Event, rule dbgen.AutomationRule,
	item dbgen.GetItemForAutomationRow, a Action) (string, error) {
	// Daftar izin diperiksa ulang: host bisa sudah dicabut sejak aturan dibuat.
	target, err := w.Webhook.ValidateURL(a.URL)
	if err != nil {
		return "", err
	}
	info, err := q.GetItemContext(ctx, item.ID)
	if err != nil {
		return "", fmt.Errorf("baca konteks item: %w", err)
	}
	var p webhookPayload
	p.Jenis, p.Waktu = "jwm.otomasi", w.Now().UTC()
	p.Aturan.ID, p.Aturan.Nama = rule.ID, rule.Nama
	p.Pemicu.Jenis, p.Pemicu.ColumnID = e.Jenis, e.ColumnID
	if len(e.Sebelum) > 0 {
		p.Pemicu.Sebelum = e.Sebelum
	}
	if len(e.Sesudah) > 0 {
		p.Pemicu.Sesudah = e.Sesudah
	}
	p.Item.ID, p.Item.Judul = item.ID, info.Judul
	if base := strings.TrimRight(w.Webhook.BaseURL, "/"); base != "" {
		p.Item.URL = base + "/board/" + info.BoardID.String()
	}
	p.Board = ref{info.BoardID, info.BoardNama}
	p.Proyek = ref{info.ProjectID, info.ProjectNama}
	body, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("susun payload: %w", err)
	}
	ruleID := rule.ID
	if _, err := q.EnqueueWebhook(ctx, dbgen.EnqueueWebhookParams{RuleID: &ruleID, Url: target, Payload: body}); err != nil {
		return "", fmt.Errorf("antrekan webhook: %w", err)
	}
	return "webhook diantrekan", nil
}

// DispatchWebhooks mengirim kiriman yang jatuh tempo. Kiriman yang gagal dicoba
// ulang 3 kali (jeda 30 detik, 2 menit, 10 menit), lalu ditandai gagal.
// Mengembalikan jumlah kiriman yang diproses.
func (w *Worker) DispatchWebhooks(ctx context.Context) (int, error) {
	rows, err := w.q.ClaimWebhooks(ctx, dispatchBatch)
	if err != nil {
		return 0, fmt.Errorf("ambil kiriman: %w", err)
	}
	for _, r := range rows {
		code, sendErr := w.sendOne(ctx, r)
		status, galat, next := DeliveryOK, "", w.Now()
		var kode *int32
		if code != 0 {
			c := int32(code)
			kode = &c
		}
		if sendErr != nil {
			galat = sendErr.Error()
			attempt := int(r.Percobaan) + 1
			if attempt >= WebhookAttempts || errors.Is(sendErr, errPermanent) {
				status = DeliveryFailed
				w.log.Warn("otomasi: webhook gagal permanen", "id", r.ID, "err", sendErr)
			} else {
				status, next = DeliveryPending, w.Now().Add(webhookBackoff[attempt-1])
			}
		}
		if err := w.q.FinishWebhook(ctx, dbgen.FinishWebhookParams{
			ID: r.ID, Status: status, Galat: galat, KodeHttp: kode, Berikutnya: next,
		}); err != nil {
			return 0, fmt.Errorf("simpan hasil kiriman: %w", err)
		}
	}
	return len(rows), nil
}

// errPermanent: kegagalan yang tidak akan berhasil bila diulang.
var errPermanent = errors.New("tidak dapat dikirim")

func (w *Worker) sendOne(ctx context.Context, r dbgen.ClaimWebhooksRow) (int, error) {
	target, err := w.Webhook.ValidateURL(r.Url)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", errPermanent, err)
	}
	if r.RuleID == nil {
		return 0, fmt.Errorf("%w: aturan sudah dihapus", errPermanent)
	}
	secret, err := w.q.GetAutomationSecret(ctx, *r.RuleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("%w: rahasia webhook tidak ada", errPermanent)
	}
	if err != nil {
		return 0, fmt.Errorf("baca rahasia: %w", err)
	}
	return deliver(ctx, w.Client, target, secret, r.ID, r.Payload, w.Now())
}

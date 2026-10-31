// Package realtime menyalurkan notifikasi perubahan data proyek ke klien lewat
// Server-Sent Events. Sumbernya NOTIFY jwm_berubah dari trigger database
// (migrasi 00010), sehingga semua jalur penulisan tercakup tanpa mengubah layanan.
package realtime

import (
	"errors"
	"sync"

	"github.com/google/uuid"
)

// Event adalah satu perubahan: proyek dan jenisnya (item, dependensi, board,
// anggaran, proyek). Tidak berisi data; klien mengambil ulang lewat API biasa
// sehingga pemeriksaan hak tetap terjadi di sana.
type Event struct {
	ProjectID uuid.UUID `json:"project_id"`
	Jenis     string    `json:"jenis"`
}

// ErrTooManyStreams: pengguna sudah membuka batas maksimal koneksi stream.
var ErrTooManyStreams = errors.New("terlalu banyak koneksi pembaruan langsung; tutup tab lain lalu coba lagi")

// Hub membagikan event ke pelanggan menurut proyek.
type Hub struct {
	maxPerUser int

	mu      sync.Mutex
	subs    map[*Subscription]struct{}
	perUser map[uuid.UUID]int

	done     chan struct{}
	doneOnce sync.Once
}

// NewHub membuat hub dengan batas koneksi per pengguna.
func NewHub(maxPerUser int) *Hub {
	return &Hub{
		maxPerUser: maxPerUser, subs: map[*Subscription]struct{}{}, perUser: map[uuid.UUID]int{},
		done: make(chan struct{}),
	}
}

// Subscription adalah satu koneksi pelanggan.
type Subscription struct {
	hub      *Hub
	userID   uuid.UUID
	projects map[uuid.UUID]bool
	ch       chan Event
	once     sync.Once
}

// Subscribe mendaftarkan pelanggan untuk proyek-proyek tersebut.
func (h *Hub) Subscribe(userID uuid.UUID, projects []uuid.UUID) (*Subscription, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.perUser[userID] >= h.maxPerUser {
		return nil, ErrTooManyStreams
	}
	s := &Subscription{hub: h, userID: userID, ch: make(chan Event, 64)}
	s.setProjectsLocked(projects)
	h.subs[s] = struct{}{}
	h.perUser[userID]++
	return s, nil
}

func (s *Subscription) setProjectsLocked(projects []uuid.UUID) {
	m := make(map[uuid.UUID]bool, len(projects))
	for _, p := range projects {
		m[p] = true
	}
	s.projects = m
}

// SetProjects mengganti daftar proyek (mis. setelah hak akses diperiksa ulang).
func (s *Subscription) SetProjects(projects []uuid.UUID) {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	s.setProjectsLocked(projects)
}

// C adalah kanal event pelanggan.
func (s *Subscription) C() <-chan Event { return s.ch }

// Close melepas pelanggan. Aman dipanggil berulang.
func (s *Subscription) Close() {
	s.once.Do(func() {
		h := s.hub
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.subs, s)
		if h.perUser[s.userID]--; h.perUser[s.userID] <= 0 {
			delete(h.perUser, s.userID)
		}
	})
}

// Publish mengirim event ke pelanggan proyek itu tanpa menunggu. Bila antrean
// pelanggan penuh, event dibuang: klien yang tertinggal tetap akan memuat
// ulang karena event lain untuk proyek yang sama masih menunggu di antrean.
func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		if !s.projects[e.ProjectID] {
			continue
		}
		select {
		case s.ch <- e:
		default:
		}
	}
}

// Count mengembalikan jumlah pelanggan aktif (untuk tes dan metrik).
func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// Shutdown menutup semua stream (dipasang lewat http.Server.RegisterOnShutdown
// agar shutdown tidak menunggu koneksi berumur panjang).
func (h *Hub) Shutdown() { h.doneOnce.Do(func() { close(h.done) }) }

// Done tertutup setelah Shutdown.
func (h *Hub) Done() <-chan struct{} { return h.done }

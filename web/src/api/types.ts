// Tipe data API (lihat docs/openapi.yaml).

import type { Density } from '../density'

export type { Density }

export type Role = 'admin' | 'pmo' | 'project_manager' | 'member' | 'viewer'
export type ProjectRole = 'manager' | 'member' | 'viewer'
export type Priority = 'rendah' | 'sedang' | 'tinggi' | 'kritis'

export const ROLE_LABEL: Record<Role, string> = {
  admin: 'Admin',
  pmo: 'PMO',
  project_manager: 'Project Manager',
  member: 'Member',
  viewer: 'Viewer',
}

export const PROJECT_ROLE_LABEL: Record<ProjectRole, string> = {
  manager: 'Manager',
  member: 'Member',
  viewer: 'Viewer',
}

export const PRIORITIES: Priority[] = ['rendah', 'sedang', 'tinggi', 'kritis']

export interface SessionUser {
  id: string
  nama: string
  email: string
  peran: Role
  wajib_ganti_sandi: boolean
  kepadatan: Density
}

export interface SessionResponse {
  user: SessionUser
  csrf_token: string
}

export interface AdminUser {
  id: string
  nama: string
  email: string
  peran: Role
  aktif: boolean
  wajib_ganti_sandi: boolean
  terkunci_hingga: string | null
  login_terakhir: string | null
  dibuat_pada: string
}

export interface UserWithTempPassword {
  user: AdminUser
  sandi_sementara: string
}

export interface UserSummary {
  id: string
  nama: string
  email: string
}

export interface Project {
  id: string
  nama: string
  pemilik_id: string
  pemilik_nama: string
  mulai: string | null
  selesai: string | null
  prioritas: Priority
  dibuat_pada: string
  diubah_pada: string
  diarsipkan_pada: string | null
  peran_saya: ProjectRole | ''
  portfolio_id: string | null
  /** Hanya pada GET /projects/{id}. */
  portfolio_nama?: string | null
}

export interface Portfolio {
  id: string
  nama: string
  jumlah_proyek: number
  dibuat_pada: string
  diubah_pada: string
}

export type Health = 'hijau' | 'kuning' | 'merah'

export const HEALTH_STYLE: Record<Health, { judul: string; warna: string }> = {
  hijau: { judul: 'Hijau', warna: 'hijau' },
  kuning: { judul: 'Kuning', warna: 'kuning' },
  merah: { judul: 'Merah', warna: 'merah' },
}

export type RiskRule =
  | 'lewat_tenggat'
  | 'tenggat_dekat'
  | 'tanpa_penanggung_jawab'
  | 'predecessor_terlambat'
  | 'melewati_tenggat_proyek'

/** Urutan dan label aturan risiko (SPEC bagian 5). */
export const RISK_RULES: { kode: RiskRule; label: string }[] = [
  { kode: 'lewat_tenggat', label: 'Lewat tenggat' },
  { kode: 'tenggat_dekat', label: 'Tenggat dekat, progres rendah' },
  { kode: 'tanpa_penanggung_jawab', label: 'Tanpa penanggung jawab' },
  { kode: 'predecessor_terlambat', label: 'Predecessor terlambat (jalur kritis)' },
  { kode: 'melewati_tenggat_proyek', label: 'Slack ≤ 0 terhadap tenggat proyek' },
]

export interface ManualStatus {
  status: Health
  alasan: string
  oleh_nama: string
  pada: string
}

export interface ProjectHealth {
  id: string
  nama: string
  pemilik_id: string
  pemilik_nama: string
  prioritas: Priority
  mulai: string | null
  selesai: string | null
  portfolio_id: string | null
  portfolio_nama: string | null
  progres: number | null
  progres_rencana: number | null
  progres_jadwal: number | null
  jumlah_item: number
  item_berisiko: number
  per_aturan: Record<RiskRule, number>
  kritis_terlambat: number
  health: Health
  health_otomatis: Health
  health_alasan: string[]
  status_manual: ManualStatus | null
  peringatan: string[]
}

export interface PortfolioDashboard {
  items: ProjectHealth[]
  ambang: {
    tenggat_dekat_hari: number
    tenggat_dekat_progres: number
    kuning_persen: number
    merah_persen: number
    tertinggal_poin: number
  }
  hari_ini: string
}

export interface RiskItem {
  item_id: string
  board_id: string
  board_nama: string
  judul: string
  kritis: boolean
  alasan: { kode: RiskRule; pesan: string }[]
}

export interface Member {
  user_id: string
  nama: string
  email: string
  peran_global: Role
  aktif: boolean
  peran_proyek: ProjectRole
  peran_efektif: ProjectRole | ''
  ditambahkan_pada: string
}

export type ColumnType =
  | 'text'
  | 'status'
  | 'person'
  | 'date'
  | 'timeline'
  | 'number'
  | 'progress'
  | 'priority'
  | 'dependency'

export interface StatusLabel {
  id: string
  judul: string
  warna: string
}

export interface Column {
  id: string
  board_id: string
  tipe: ColumnType
  judul: string
  urutan: number
  konfigurasi: { labels?: StatusLabel[]; satuan?: string }
}

export interface Group {
  id: string
  board_id: string
  judul: string
  warna: string
  urutan: number
}

export interface Board {
  id: string
  project_id: string
  nama: string
  urutan: number
}

export interface BoardDetail extends Board {
  kolom: Column[]
  grup: Group[]
  peran_saya: ProjectRole | ''
}

export interface Item {
  id: string
  board_id: string
  group_id: string
  judul: string
  urutan: number
  dibuat_oleh: string
  dibuat_pada: string
  diubah_pada: string
  nilai: Record<string, unknown>
  bisa_ubah: boolean
  bisa_hapus: boolean
  /** Hanya pada respons ubah nilai: successor yang ikut digeser otomatis. */
  geser_otomatis?: ShiftedItem[]
}

export interface ShiftedItem {
  id: string
  judul: string
  mulai: string
  selesai: string
  predecessor_id: string
}

/** Hasil CPM satu item (GET /boards/{id}/schedule). Tanggal YYYY-MM-DD. */
export interface ItemSchedule {
  item_id: string
  es: string | null
  ef: string | null
  ls: string | null
  lf: string | null
  /** Slack dalam hari kerja. */
  slack: number | null
  kritis: boolean
  melanggar_fs: boolean
  tanpa_jadwal: boolean
}

export interface ScheduleWarning {
  kode: 'kalender_kosong'
  tahun?: number
  pesan: string
}

export interface ScheduleView {
  kolom_id: string
  kolom_utama: boolean
  selesai_proyek: string | null
  items: ItemSchedule[]
  dependensi_dilanggar: string[]
  peringatan: ScheduleWarning[]
}

export interface CalendarDay {
  tanggal: string
  hari_kerja: boolean
  keterangan: string
}

export interface CalendarYear {
  tahun: number
  items: CalendarDay[]
  tahun_terisi: number[]
}

export interface Dependency {
  id: string
  board_id: string
  predecessor_id: string
  successor_id: string
  tipe: 'FS'
  lag_hari: number
}

export interface Page<T> {
  items: T[]
  kursor_berikut?: string | null
}

export interface StatusSlice {
  label: string
  warna: string
  jumlah: number
}

export interface Workload {
  user_id: string
  nama: string
  item_terbuka: number
  /** Jumlah durasi hari kerja timeline utama item terbuka. */
  hari_kerja: number
  terlambat: number
}

export interface WeekCount {
  /** Senin awal minggu, YYYY-MM-DD. */
  minggu_mulai: string
  selesai: number
}

/** Jumlah uang berupa string desimal (mis. "1500000.50") agar tidak kehilangan presisi. */
export interface Finance {
  project_id: string
  nama: string
  anggaran: string
  realisasi: string
  sisa: string
  jumlah_entri: number
}

export interface DashboardPanels {
  project_id: string | null
  hari_ini: string
  jumlah_item: number
  status: StatusSlice[]
  tanpa_status: number
  beban: Workload[]
  belum_ditugaskan: number
  tren: WeekCount[]
  keuangan: Finance[]
}

export type BudgetKind = 'anggaran' | 'realisasi'

export interface BudgetEntry {
  id: string
  project_id: string
  tanggal: string
  jenis: BudgetKind
  jumlah: string
  keterangan: string
  dibuat_oleh_nama: string
  dibuat_pada: string
  diubah_pada: string
}

export interface Budget {
  items: BudgetEntry[]
  total: { anggaran: string; realisasi: string; sisa: string }
  bisa_ubah: boolean
}

// Tipe data API (lihat docs/openapi.yaml).

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
}

export interface Page<T> {
  items: T[]
  kursor_berikut?: string | null
}

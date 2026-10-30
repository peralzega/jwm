import { Link } from 'react-router'

export function NotFoundPage() {
  return (
    <div className="gap-d flex flex-col">
      <h1 className="text-(length:--heading-size) font-semibold">Halaman tidak ditemukan</h1>
      <p className="text-muted-foreground">Halaman tidak ada, atau Anda tidak punya akses.</p>
      <Link to="/" className="text-primary underline">
        Kembali ke daftar proyek
      </Link>
    </div>
  )
}

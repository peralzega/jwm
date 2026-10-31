// Ekspor CSV di browser tanpa pustaka. Pemisah titik koma dan BOM UTF-8 agar
// langsung terbaca Excel berlokal Indonesia.

export type Cell = string | number

/** Satu sel CSV. Teks diawali = + - @ (rumus spreadsheet) diberi petik tunggal. */
export function csvCell(v: Cell): string {
  if (typeof v === 'number') return Number.isFinite(v) ? String(v) : ''
  let s = v
  if (/^[=+\-@\t\r]/.test(s)) s = `'${s}`
  return /[";\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}

export function toCSV(header: string[], rows: Cell[][]): string {
  return [header, ...rows].map((r) => r.map(csvCell).join(';')).join('\r\n') + '\r\n'
}

/** Mengunduh teks CSV sebagai berkas. */
export function downloadCSV(filename: string, header: string[], rows: Cell[][]) {
  const blob = new Blob([String.fromCharCode(0xfeff), toCSV(header, rows)], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

const rupiah = new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', minimumFractionDigits: 0, maximumFractionDigits: 2 })

/** Jumlah desimal (string dari API) sebagai Rupiah, mis. "Rp 1.500.000,5". */
export function formatRupiah(s: string): string {
  const n = Number(s)
  return Number.isFinite(n) ? rupiah.format(n) : s
}

import { describe, expect, it } from 'vitest'

import { csvCell, formatRupiah, toCSV } from './export'

describe('CSV', () => {
  it.each<[string | number, string]>([
    ['Teks biasa', 'Teks biasa'],
    [42, '42'],
    [-1.5, '-1.5'],
    [Number.NaN, ''],
    ['a;b', '"a;b"'],
    ['kata "kutip"', '"kata ""kutip"""'],
    ['baris\nbaru', '"baris\nbaru"'],
    // Rumus spreadsheet dinetralkan (CSV injection).
    ['=HYPERLINK("x")', `"'=HYPERLINK(""x"")"`],
    ['+62 812', "'+62 812"],
    ['-sum', "'-sum"],
    ['@cmd', "'@cmd"],
  ])('%j -> %s', (v, want) => expect(csvCell(v)).toBe(want))

  it('baris dipisah CRLF, kolom titik koma', () => {
    expect(toCSV(['Nama', 'Jumlah'], [['Budi', 3], ['Sari; Dewi', 1]])).toBe('Nama;Jumlah\r\nBudi;3\r\n"Sari; Dewi";1\r\n')
  })
})

describe('formatRupiah', () => {
  it('memformat string desimal sebagai Rupiah', () => {
    expect(formatRupiah('1500000.50').replace(/\s/g, ' ')).toBe('Rp 1.500.000,5')
    expect(formatRupiah('-499999.50').replace(/\s/g, ' ')).toMatch(/^-Rp 499\.999,5$/)
    expect(formatRupiah('bukan')).toBe('bukan')
  })
})

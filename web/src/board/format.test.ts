import { describe, expect, it } from 'vitest'

import { formatDate, formatNumber, formatTimeline, initials, parseNumber, parseProgress, parseTimeline } from './format'

describe('parseNumber', () => {
  const cases: [string, number | null | 'galat'][] = [
    ['', null],
    ['  ', null],
    ['12', 12],
    ['-4', -4],
    ['3,5', 3.5],
    ['3.5', 3.5],
    ['1 000', 1000],
    ['abc', 'galat'],
    ['1,2,3', 'galat'],
    ['1e3', 'galat'],
    ['9999999999999999', 'galat'],
  ]
  for (const [input, want] of cases) {
    it(`"${input}"`, () => {
      const p = parseNumber(input)
      if (want === 'galat') expect(p.ok).toBe(false)
      else expect(p).toEqual({ ok: true, value: want })
    })
  }
})

describe('parseProgress', () => {
  const cases: [string, number | null | 'galat'][] = [
    ['', null],
    ['0', 0],
    ['100', 100],
    ['45%', 45],
    ['101', 'galat'],
    ['-1', 'galat'],
    ['2,5', 'galat'],
  ]
  for (const [input, want] of cases) {
    it(`"${input}"`, () => {
      const p = parseProgress(input)
      if (want === 'galat') expect(p.ok).toBe(false)
      else expect(p).toEqual({ ok: true, value: want })
    })
  }
})

describe('parseTimeline', () => {
  it('keduanya kosong mengosongkan sel', () => {
    expect(parseTimeline('', '')).toEqual({ ok: true, value: null })
  })
  it('menerima rentang yang sah', () => {
    expect(parseTimeline('2026-10-01', '2026-10-15')).toEqual({ ok: true, value: { mulai: '2026-10-01', selesai: '2026-10-15' } })
  })
  it('menolak selesai sebelum mulai dan tanggal yang tidak lengkap', () => {
    expect(parseTimeline('2026-10-15', '2026-10-01').ok).toBe(false)
    expect(parseTimeline('2026-10-15', '').ok).toBe(false)
  })
})

describe('format', () => {
  it('tanggal tidak bergeser zona waktu dan string tidak sah menjadi kosong', () => {
    expect(formatDate('2026-01-01')).toMatch(/^1 Jan(uari)? 2026$/)
    expect(formatDate('kemarin')).toBe('')
    expect(formatDate(null)).toBe('')
  })
  it('timeline ringkas', () => {
    expect(formatTimeline({ mulai: '2026-10-01', selesai: '2026-10-15' })).toMatch(/^1 Okt – 15 Okt$/)
    expect(formatTimeline(null)).toBe('')
  })
  it('angka memakai format Indonesia dan satuan', () => {
    expect(formatNumber(1234.5, 'jam')).toBe('1.234,5 jam')
    expect(formatNumber(null)).toBe('')
  })
  it('inisial', () => {
    expect(initials('Budi Santoso')).toBe('BS')
    expect(initials('Sari')).toBe('SA')
    expect(initials('  ')).toBe('?')
  })
})

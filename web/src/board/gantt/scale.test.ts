import { describe, expect, it } from 'vitest'

import {
  applyDrag,
  barBox,
  chartRange,
  dayNum,
  daysFromPx,
  describeSpan,
  headerTicks,
  isoOf,
  parseScale,
  todayNum,
  weekday,
  xOf,
  type DragMode,
  type GanttScale,
  type Span,
} from './scale'

const d = dayNum

describe('nomor hari', () => {
  it.each(['1970-01-01', '2024-02-29', '2026-03-29', '2026-10-25', '2026-12-31', '2027-01-01'])('%s bolak-balik', (iso) => {
    expect(isoOf(dayNum(iso))).toBe(iso)
  })
  it('selisih hari tidak terpengaruh pergantian jam musim', () => {
    expect(d('2026-03-30') - d('2026-03-28')).toBe(2)
    expect(d('2026-11-02') - d('2026-10-31')).toBe(2)
  })
  it('hari ini memakai tanggal lokal', () => {
    expect(isoOf(todayNum(new Date(2026, 9, 31, 23, 59)))).toBe('2026-10-31')
  })
  it('weekday: 0 Minggu sampai 6 Sabtu', () => {
    expect(weekday(d('2026-11-01'))).toBe(0)
    expect(weekday(d('2026-11-02'))).toBe(1)
    expect(weekday(d('2026-10-31'))).toBe(6)
  })
})

describe('parseScale', () => {
  it.each<[string | null, GanttScale]>([
    ['hari', 'hari'],
    ['bulan', 'bulan'],
    ['minggu', 'minggu'],
    [null, 'minggu'],
    ['tahun', 'minggu'],
  ])('%s -> %s', (v, want) => expect(parseScale(v)).toBe(want))
})

describe('chartRange', () => {
  const today = d('2026-10-31') // Sabtu

  it('hari/minggu: mulai Senin, akhir eksklusif Senin, mencakup jadwal dan hari ini', () => {
    const spans: Span[] = [{ start: d('2026-09-10'), end: d('2026-09-20') }]
    for (const scale of ['hari', 'minggu'] as const) {
      const r = chartRange(spans, today, scale)
      expect(weekday(r.start)).toBe(1)
      expect(weekday(r.end)).toBe(1)
      expect(r.start).toBeLessThanOrEqual(d('2026-09-10') - 7)
      expect(r.end).toBeGreaterThan(today)
    }
  })

  it('bulan: mulai tanggal 1, akhir tanggal 1 bulan berikutnya', () => {
    const r = chartRange([{ start: d('2026-08-15'), end: d('2027-02-03') }], today, 'bulan')
    expect(isoOf(r.start)).toBe('2026-08-01')
    expect(isoOf(r.end).endsWith('-01')).toBe(true)
    expect(r.end).toBeGreaterThan(d('2027-02-03'))
  })

  it('board tanpa jadwal tetap punya sumbu minimal sekitar hari ini', () => {
    const r = chartRange([], today, 'minggu')
    expect(r.end - r.start).toBeGreaterThanOrEqual(56)
    expect(today).toBeGreaterThan(r.start)
    expect(today).toBeLessThan(r.end)
  })
})

describe('geometri', () => {
  const range = { start: d('2026-11-02'), end: d('2026-12-07') }
  it.each<[string, string, number, { left: number; width: number }]>([
    ['2026-11-02', '2026-11-02', 10, { left: 0, width: 10 }],
    ['2026-11-04', '2026-11-08', 10, { left: 20, width: 50 }],
    ['2026-11-04', '2026-11-08', 4, { left: 8, width: 20 }],
  ])('%s s.d. %s, %i px/hari', (a, b, dayW, want) => {
    expect(barBox({ start: d(a), end: d(b) }, range, dayW)).toEqual(want)
  })
  it('xOf di luar rentang boleh negatif', () => {
    expect(xOf(d('2026-11-01'), range, 12)).toBe(-12)
  })
  it.each<[number, number, number]>([
    [0, 12, 0],
    [5, 12, 0],
    [6, 12, 1],
    [-6, 12, -1],
    [-7, 12, -1],
    [37, 12, 3],
    [-100, 28, -4],
  ])('daysFromPx(%i, %i) = %i', (dx, dayW, want) => expect(daysFromPx(dx, dayW)).toBe(want))
})

describe('applyDrag', () => {
  const s = { start: 100, end: 104 }
  it.each<[DragMode, number, Span]>([
    ['geser', 3, { start: 103, end: 107 }],
    ['geser', -2, { start: 98, end: 102 }],
    ['mulai', -2, { start: 98, end: 104 }],
    ['mulai', 3, { start: 103, end: 104 }],
    ['mulai', 9, { start: 104, end: 104 }],
    ['selesai', 2, { start: 100, end: 106 }],
    ['selesai', -3, { start: 100, end: 101 }],
    ['selesai', -9, { start: 100, end: 100 }],
  ])('%s %i', (mode, delta, want) => expect(applyDrag(s, mode, delta)).toEqual(want))
})

describe('headerTicks', () => {
  const range = { start: d('2026-10-26'), end: d('2026-11-09') } // 2 minggu, melintasi bulan
  it('hari: satu tick per hari, bulan di baris atas dipotong ke rentang', () => {
    const { top, bottom } = headerTicks(range, 'hari', 10)
    expect(bottom).toHaveLength(14)
    expect(bottom[0]).toMatchObject({ left: 0, width: 10, label: '26' })
    expect(top.map((t) => [t.left, t.width])).toEqual([
      [0, 60],
      [60, 80],
    ])
    expect(top[0]?.label).toMatch(/Oktober 2026/)
  })
  it('minggu: satu tick per 7 hari', () => {
    const { bottom } = headerTicks(range, 'minggu', 10)
    expect(bottom.map((t) => [t.left, t.width])).toEqual([
      [0, 70],
      [70, 70],
    ])
  })
  it('bulan: tahun di atas, bulan di bawah', () => {
    const r = { start: d('2026-11-01'), end: d('2027-03-01') }
    const { top, bottom } = headerTicks(r, 'bulan', 1)
    expect(top.map((t) => t.label)).toEqual(['2026', '2027'])
    expect(bottom).toHaveLength(4)
    expect(bottom.reduce((n, t) => n + t.width, 0)).toBe(r.end - r.start)
  })
})

it('describeSpan menyebut tanggal dan durasi inklusif', () => {
  expect(describeSpan({ start: d('2026-11-02'), end: d('2026-11-06') })).toMatch(/^2 Nov 2026 – 6 Nov 2026, 5 hari$/)
})

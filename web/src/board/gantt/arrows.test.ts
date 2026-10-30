import { describe, expect, it } from 'vitest'

import { arrowPath, buildArrows } from './arrows'

describe('arrowPath', () => {
  it('successor jauh di kanan: siku sederhana', () => {
    expect(arrowPath(100, 16, 160, 48, 8, 32)).toBe('M100 16H108V48H160')
  })
  it('successor mulai sebelum predecessor selesai: memutar lewat celah baris', () => {
    expect(arrowPath(100, 16, 60, 48, 8, 32)).toBe('M100 16H108V32H52V48H60')
  })
  it('successor di atas predecessor: celah di atas baris asal', () => {
    expect(arrowPath(100, 48, 60, 16, 8, 32)).toBe('M100 48H108V32H52V16H60')
  })
})

describe('buildArrows', () => {
  const range = { start: 0, end: 100 }
  const geo = { range, dayW: 10, rowH: 32, gap: 8 }
  const spans = new Map([
    ['a', { start: 0, end: 4 }],
    ['b', { start: 10, end: 12 }],
    ['c', { start: 3, end: 6 }],
  ])
  const rows = new Map([
    ['a', 1],
    ['b', 2],
    ['c', 4],
  ])

  it('menandai pelanggaran FS dan melewati item tanpa jadwal atau baris tersembunyi', () => {
    const arrows = buildArrows(
      [
        { id: 'ab', predecessor_id: 'a', successor_id: 'b' },
        { id: 'ac', predecessor_id: 'a', successor_id: 'c' },
        { id: 'ax', predecessor_id: 'a', successor_id: 'tanpa-jadwal' },
        { id: 'bc', predecessor_id: 'b', successor_id: 'c' },
      ],
      spans,
      rows,
      geo,
    )
    expect(arrows.map((a) => [a.id, a.violated])).toEqual([
      ['ab', false],
      ['ac', true],
      ['bc', true],
    ])
    // a berakhir di x=50 (hari 0..4), b mulai di x=100; pusat baris 1 = 48, baris 2 = 80.
    expect(arrows[0]?.d).toBe('M50 48H58V80H100')
    expect(arrows[0]).toMatchObject({ top: 16, bottom: 112 })
  })

  it('successor tepat sehari setelah predecessor selesai tidak melanggar', () => {
    const [a] = buildArrows(
      [{ id: 'x', predecessor_id: 'p', successor_id: 's' }],
      new Map([
        ['p', { start: 0, end: 4 }],
        ['s', { start: 5, end: 6 }],
      ]),
      new Map([
        ['p', 0],
        ['s', 1],
      ]),
      geo,
    )
    expect(a?.violated).toBe(false)
  })
})

import { describe, expect, it } from 'vitest'

// Semua ukuran tampilan lewat token di index.css (CLAUDE.md: "Gunakan token,
// bukan angka mentah"). Tes ini menolak kelas Tailwind berukuran tetap di
// komponen: skala angka (gap-2, ring-2, size-5), px (-ml-px), skala bernama
// (text-lg, max-w-sm), dan nilai arbitrer (text-[0.7rem]). Yang boleh:
// nilai 0 (top-0), kelas token (px-d, row-h), dan var()/calc(var()).
const sources = import.meta.glob(['./**/*.tsx', '!./**/*.test.tsx'], {
  query: '?raw',
  import: 'default',
  eager: true,
}) as Record<string, string>

const PROPS = [
  'p', 'px', 'py', 'pt', 'pb', 'pl', 'pr', 'm', 'mx', 'my', 'mt', 'mb', 'ml', 'mr',
  'gap', 'gap-x', 'gap-y', 'h', 'w', 'min-w', 'min-h', 'max-w', 'max-h', 'size',
  'text', 'top', 'left', 'right', 'bottom', 'inset', 'space-x', 'space-y', 'translate-x', 'translate-y',
  'ring', 'border', 'border-t', 'border-r', 'border-b', 'border-l', 'border-x', 'border-y',
  'outline', 'outline-offset', 'rounded-[a-z]+',
].join('|')
// Pecahan (top-1/2, w-1/3) bersifat relatif terhadap induk, bukan angka mentah.
const VALUE = String.raw`(?:(?!0\b)(?!\d+\/\d+)\d[\d.]*|px\b|(?:xs|sm|md|lg|xl|[2-9]xl)\b|\[(?!calc\(var|var\())`
export const FIXED = new RegExp(String.raw`(?<![\w-])-?(?:${PROPS})-${VALUE}`, 'g')

/** Teks kelas dari className="...", className={`...`}, dan konstanta kelas (const X = '...'). */
export function classText(src: string): string {
  const attrs = [...src.matchAll(/className=(?:"([^"]*)"|\{`([^`]*)`\})/g)].map((m) => m[1] ?? m[2] ?? '')
  const consts = [...src.matchAll(/^const [A-Z_]+ =\s*'([^']*)'/gm)].map((m) => m[1] ?? '')
  return [...attrs, ...consts].join(' ')
}

describe('pola penjaga', () => {
  it('menangkap ukuran tetap', () => {
    const bad = 'gap-2 ring-2 -mb-px size-5 text-[0.7rem] max-h-72 border-b-2 text-lg max-w-sm focus-visible:outline-2 outline-offset-1 mt-24 h-1.5'
    expect(bad.match(FIXED)).toHaveLength(13)
  })
  it('membolehkan token, nol, dan var()', () => {
    const ok =
      'gap-(--space-sm) top-0 left-0 px-d row-h min-w-0 flex-1 ring-(length:--focus-ring-w) -mb-(--line-w) ' +
      'w-[calc(var(--kanban-col-w)_-_2*var(--density-pad-x))] text-(length:--heading-size) border border-line ' +
      'rounded rounded-full text-muted-foreground text-right text-white outline-none max-w-(--auth-card-w) top-1/2 -translate-y-1/2'
    expect(ok.match(FIXED)).toBeNull()
  })
})

describe('komponen memakai token ukuran', () => {
  const files = Object.entries(sources)

  it('semua komponen ikut diperiksa', () => {
    const names = files.map(([f]) => f)
    expect(names.length).toBeGreaterThan(20)
    expect(names).toEqual(expect.arrayContaining(['./components/ui.tsx', './board/BoardTable.tsx', './pages/LoginPage.tsx']))
  })

  for (const [file, src] of files) {
    it(file, () => {
      expect(classText(src).match(FIXED) ?? []).toEqual([])
    })
  }
})

import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

import { formatNotices, isAllowed, packageNameFromPath, readPackageNotice } from './third-party-notices'

describe('packageNameFromPath', () => {
  const cases: [string, string | null][] = [
    ['C:\\proj\\node_modules\\clsx\\dist\\clsx.mjs', 'clsx'],
    ['/p/node_modules/@radix-ui/react-popover/dist/index.mjs', '@radix-ui/react-popover'],
    ['/p/node_modules/a/node_modules/b/index.js?commonjs-proxy', 'b'],
    ['/p/src/main.tsx', null],
  ]
  for (const [id, want] of cases) {
    it(id, () => expect(packageNameFromPath(id)).toBe(want))
  }
})

describe('isAllowed', () => {
  it.each([
    ['MIT', true],
    ['Apache-2.0', true],
    ['0BSD', true],
    ['(MIT OR GPL-3.0)', true],
    ['GPL-3.0', false],
    ['AGPL-3.0-only', false],
    ['UNLICENSED', false],
    ['TIDAK DIKETAHUI', false],
  ])('%s -> %s', (license, want) => expect(isAllowed(license)).toBe(want))
})

describe('readPackageNotice dan formatNotices', () => {
  function pkgDir(files: Record<string, string>): string {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'jwm-notice-'))
    for (const [f, c] of Object.entries(files)) fs.writeFileSync(path.join(dir, f), c)
    return dir
  }

  it('menyertakan teks LICENSE dan NOTICE (Apache-2.0)', () => {
    const dir = pkgDir({
      'package.json': JSON.stringify({ name: 'cva-uji', version: '1.0.0', license: 'Apache-2.0', repository: { url: 'git+https://x/y.git' } }),
      LICENSE: 'Apache License\nVersion 2.0',
      NOTICE: 'Hak cipta pemberitahuan',
    })
    const n = readPackageNotice(dir)
    expect(n).toMatchObject({ name: 'cva-uji', license: 'Apache-2.0', repository: 'https://x/y.git' })
    const out = formatNotices([n], [{ title: 'shadcn/ui, MIT', body: 'MIT License' }])
    expect(out).toContain('cva-uji 1.0.0')
    expect(out).toContain('Apache License\nVersion 2.0')
    expect(out).toContain('--- NOTICE ---\nHak cipta pemberitahuan')
    expect(out).toContain('shadcn/ui, MIT')
  })

  it('paket MIT tanpa berkas lisensi memakai teks standar MIT dengan pemegang hak', () => {
    const dir = pkgDir({ 'package.json': JSON.stringify({ name: 'tanpa-berkas', version: '2.0.0', license: 'MIT', author: 'Contoh Penulis' }) })
    const n = readPackageNotice(dir)
    expect(n.licenseText).toContain('Copyright (c) Contoh Penulis')
    expect(n.licenseText).toContain('Permission is hereby granted')
  })
})

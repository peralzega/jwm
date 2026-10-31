// Plugin Vite: menulis dist/THIRD-PARTY-NOTICES.txt berisi nama, versi,
// lisensi, dan teks lisensi setiap paket pihak ketiga yang masuk bundel, plus
// kode pihak ketiga yang disalin ke repo (shadcn/ui). Build gagal bila ada
// paket dengan lisensi di luar daftar yang diizinkan (CLAUDE.md: tanpa GPL).

import fs from 'node:fs'
import path from 'node:path'
import type { Plugin } from 'vite'

export const ALLOWED_LICENSES = ['MIT', 'ISC', 'Apache-2.0', 'BSD-2-Clause', 'BSD-3-Clause', '0BSD'] as const

const LICENSE_FILES = /^(licen[cs]e|copying)(\.(md|txt))?$/i
const NOTICE_FILES = /^notice(\.(md|txt))?$/i

export interface PackageNotice {
  name: string
  version: string
  license: string
  repository: string
  licenseText: string
  noticeText?: string
}

/** Nama paket dari path modul di node_modules (paket terdalam bila bersarang). */
export function packageNameFromPath(id: string): string | null {
  const norm = id.replaceAll('\\', '/').split('?')[0] ?? ''
  const i = norm.lastIndexOf('/node_modules/')
  if (i < 0) return null
  const parts = norm.slice(i + '/node_modules/'.length).split('/')
  const first = parts[0]
  if (!first) return null
  return first.startsWith('@') && parts[1] ? `${first}/${parts[1]}` : first
}

export function isAllowed(license: string): boolean {
  // Ekspresi SPDX sederhana "(MIT OR Apache-2.0)": cukup salah satu diizinkan.
  return license
    .replace(/[()]/g, '')
    .split(/\s+OR\s+/i)
    .some((l) => (ALLOWED_LICENSES as readonly string[]).includes(l.trim()))
}

const MIT_TEMPLATE = (holder: string) => `MIT License

Copyright (c) ${holder}

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.`

function readFirst(dir: string, pattern: RegExp): string | undefined {
  const file = fs.readdirSync(dir).find((f) => pattern.test(f))
  return file ? fs.readFileSync(path.join(dir, file), 'utf8').trim() : undefined
}

function authorOf(pkg: Record<string, unknown>): string {
  const a = pkg.author
  if (typeof a === 'string') return a
  if (a && typeof a === 'object' && 'name' in a && typeof a.name === 'string') return a.name
  return `kontributor ${String(pkg.name)}`
}

/** Membaca pemberitahuan satu paket dari direktorinya. */
export function readPackageNotice(dir: string): PackageNotice {
  const pkg = JSON.parse(fs.readFileSync(path.join(dir, 'package.json'), 'utf8')) as Record<string, unknown>
  const license = typeof pkg.license === 'string' ? pkg.license : 'TIDAK DIKETAHUI'
  const repo = pkg.repository
  const repository =
    typeof repo === 'string' ? repo : repo && typeof repo === 'object' && 'url' in repo ? String(repo.url) : String(pkg.homepage ?? '')
  let licenseText = readFirst(dir, LICENSE_FILES)
  if (!licenseText && license === 'MIT') licenseText = `(Paket tidak menyertakan berkas lisensi; teks standar MIT.)\n\n${MIT_TEMPLATE(authorOf(pkg))}`
  return {
    name: String(pkg.name),
    version: String(pkg.version),
    license,
    repository: repository.replace(/^git\+/, ''),
    licenseText: licenseText ?? '(Teks lisensi tidak ditemukan di paket.)',
    noticeText: readFirst(dir, NOTICE_FILES),
  }
}

export function formatNotices(notices: PackageNotice[], vendored: { title: string; body: string }[]): string {
  const line = '='.repeat(78)
  const header = [
    'JWM: pemberitahuan perangkat lunak pihak ketiga (frontend web)',
    'Berkas ini dibuat otomatis saat build (web/build/third-party-notices.ts).',
    `Paket dalam bundel: ${notices.length}. Kode pihak ketiga yang disalin ke repo: ${vendored.length}.`,
  ].join('\n')
  const parts = [
    ...vendored.map((v) => `${line}\n${v.title}\n${line}\n\n${v.body}`),
    ...notices.map((n) =>
      [`${line}`, `${n.name} ${n.version}`, `Lisensi: ${n.license}`, `Sumber: ${n.repository}`, line, '', n.licenseText, n.noticeText ? `\n--- NOTICE ---\n${n.noticeText}` : '']
        .join('\n')
        .trimEnd(),
    ),
  ]
  return `${header}\n\n${parts.join('\n\n')}\n`
}

/**
 * @param extraPackages paket yang masuk ke keluaran tetapi bukan lewat modul JS
 *   (mis. CSS yang diimpor dari index.css lewat Tailwind).
 * @param vendored kode pihak ketiga yang disalin ke repo, dengan teks lisensinya.
 */
export function thirdPartyNotices(opts: { root: string; extraPackages: string[]; vendored: { title: string; licenseFile: string }[] }): Plugin {
  return {
    name: 'jwm-third-party-notices',
    apply: 'build',
    generateBundle(_options, bundle) {
      const names = new Set<string>(opts.extraPackages)
      for (const out of Object.values(bundle)) {
        if (out.type !== 'chunk') continue
        for (const id of Object.keys(out.modules)) {
          const name = packageNameFromPath(id)
          if (name) names.add(name)
        }
      }
      const notices = [...names].sort().map((n) => readPackageNotice(path.join(opts.root, 'node_modules', n)))
      const bad = notices.filter((n) => !isAllowed(n.license))
      if (bad.length) {
        this.error(`Lisensi tidak diizinkan dalam bundel: ${bad.map((b) => `${b.name}@${b.version} (${b.license})`).join(', ')}`)
      }
      const vendored = opts.vendored.map((v) => ({ title: v.title, body: fs.readFileSync(path.join(opts.root, v.licenseFile), 'utf8').trim() }))
      this.emitFile({ type: 'asset', fileName: 'THIRD-PARTY-NOTICES.txt', source: formatNotices(notices, vendored) })
    },
  }
}

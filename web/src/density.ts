import { useSyncExternalStore } from 'react'

// Kepadatan tampilan: compact (default) atau normal. Sumber kebenaran adalah
// preferensi pengguna di server (/auth/me); localStorage hanya cache agar
// halaman pertama langsung memakai kepadatan terakhir sebelum /auth/me selesai.

export type Density = 'compact' | 'normal'

const KEY = 'jwm.density'
const listeners = new Set<() => void>()

export function getDensity(): Density {
  return document.documentElement.dataset.density === 'normal' ? 'normal' : 'compact'
}

function cached(): Density {
  try {
    return localStorage.getItem(KEY) === 'normal' ? 'normal' : 'compact'
  } catch {
    return 'compact'
  }
}

/** Menerapkan kepadatan pada <html> dan memberi tahu pelanggan (mis. tabel virtual). */
export function applyDensity(d: Density = cached()): void {
  const changed = document.documentElement.dataset.density !== d
  document.documentElement.dataset.density = d
  try {
    localStorage.setItem(KEY, d)
  } catch {
    // Penyimpanan diblokir: kepadatan tetap berlaku untuk sesi ini saja.
  }
  if (changed) listeners.forEach((l) => l())
}

function subscribe(l: () => void): () => void {
  listeners.add(l)
  return () => listeners.delete(l)
}

/** Kepadatan aktif; komponen dirender ulang saat kepadatan berganti. */
export function useDensity(): Density {
  return useSyncExternalStore(subscribe, getDensity, () => 'compact')
}

/** Membaca token CSS berukuran px (mis. --density-row) sebagai angka. */
export function cssPx(name: string, fallback: number): number {
  const v = Number.parseFloat(getComputedStyle(document.documentElement).getPropertyValue(name))
  return Number.isFinite(v) && v > 0 ? v : fallback
}

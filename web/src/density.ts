// Kepadatan tampilan: compact (default) atau normal. Pilihan per peramban;
// penyimpanan per pengguna di server menyusul di M2.

export type Density = 'compact' | 'normal'

const KEY = 'jwm.density'

export function getDensity(): Density {
  try {
    return localStorage.getItem(KEY) === 'normal' ? 'normal' : 'compact'
  } catch {
    return 'compact'
  }
}

export function applyDensity(d: Density = getDensity()): void {
  document.documentElement.dataset.density = d
  try {
    localStorage.setItem(KEY, d)
  } catch {
    // Penyimpanan diblokir: kepadatan tetap berlaku untuk sesi ini saja.
  }
}

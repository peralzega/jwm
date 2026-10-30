import { describe, expect, it } from 'vitest'

import { keyAction, type Lifted } from './keyboard'

const cols = ['__tanpa_status__', 'a', 'b', 'c'] as const
const lifted = (target: string): Lifted => ({ itemId: 'x', from: 'a', target })

describe('keyAction', () => {
  const cases: { name: string; key: string; state: Lifted | null; item?: string; want: ReturnType<typeof keyAction> }[] = [
    { name: 'Spasi mengangkat', key: ' ', state: null, want: { type: 'lift' } },
    { name: 'Enter mengangkat', key: 'Enter', state: null, want: { type: 'lift' } },
    { name: 'panah tanpa kartu terangkat diabaikan', key: 'ArrowRight', state: null, want: null },
    { name: 'kanan ke kolom berikut', key: 'ArrowRight', state: lifted('a'), want: { type: 'move', to: 'b' } },
    { name: 'kiri ke "Tanpa status"', key: 'ArrowLeft', state: lifted('a'), want: { type: 'move', to: '__tanpa_status__' } },
    { name: 'tidak melewati kolom terakhir', key: 'ArrowRight', state: lifted('c'), want: null },
    { name: 'tidak melewati kolom pertama', key: 'ArrowLeft', state: lifted('__tanpa_status__'), want: null },
    { name: 'Spasi melepas di tujuan', key: ' ', state: lifted('b'), want: { type: 'drop', to: 'b' } },
    { name: 'Esc membatalkan', key: 'Escape', state: lifted('b'), want: { type: 'cancel' } },
    { name: 'Tab membatalkan', key: 'Tab', state: lifted('b'), want: { type: 'cancel' } },
    { name: 'kartu lain saat satu terangkat: Spasi mengangkat kartu itu', key: ' ', state: lifted('b'), item: 'y', want: { type: 'lift' } },
    { name: 'atas/bawah diabaikan', key: 'ArrowDown', state: lifted('b'), want: null },
  ]
  for (const tc of cases) {
    it(tc.name, () => {
      expect(keyAction(tc.key, tc.state, tc.item ?? 'x', cols)).toEqual(tc.want)
    })
  }
})

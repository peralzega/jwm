import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'

import { setCsrfToken } from '../api/client'

// jsdom tidak punya ResizeObserver; Radix (Popover, Tooltip) memakainya untuk
// mengukur elemen. Tiruan diam cukup karena jsdom tidak punya tata letak.
class ResizeObserverStub {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}
globalThis.ResizeObserver ??= ResizeObserverStub

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  setCsrfToken('')
})

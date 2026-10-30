/// <reference types="vitest/config" />
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'

import { thirdPartyNotices } from './build/third-party-notices.ts'

const root = fileURLToPath(new URL('.', import.meta.url))

// Server pengembangan meneruskan /api ke server Go agar cookie sesi tetap satu origin.
export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    // dist/THIRD-PARTY-NOTICES.txt; build gagal bila ada lisensi di luar daftar yang diizinkan.
    thirdPartyNotices({
      root,
      // Diimpor lewat index.css (Tailwind), bukan lewat modul JS.
      extraPackages: ['tw-animate-css'],
      vendored: [
        {
          title:
            'shadcn/ui (https://github.com/shadcn-ui/ui), MIT. Disalin dan diubah: web/src/components/ui/* dan varian data-open/data-closed di web/src/index.css (dari shadcn 4.21.5).',
          licenseFile: 'build/licenses/shadcn-ui.LICENSE.txt',
        },
      ],
    }),
  ],
  // Alias @ -> src (dipakai komponen shadcn/ui; lihat components.json).
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': 'http://localhost:8080',
      '/healthz': 'http://localhost:8080',
    },
  },
  build: {
    sourcemap: false,
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    css: false,
  },
})

import { clsx, type ClassValue } from 'clsx'
import { extendTailwindMerge } from 'tailwind-merge'

// Utilitas token kepadatan (index.css) didaftarkan agar tailwind-merge tahu
// bahwa px-d menggantikan px-*, row-h menggantikan min-h-*, dan seterusnya.
const twMerge = extendTailwindMerge({
  extend: {
    classGroups: {
      px: ['px-d'],
      py: ['py-d'],
      gap: ['gap-d'],
      'min-h': ['row-h'],
    },
  },
})

/** Menggabungkan kelas (shadcn/ui); kelas yang belakangan menang bila bertentangan. */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs))
}

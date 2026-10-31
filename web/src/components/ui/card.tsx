import type { ComponentProps } from 'react'

import { cn } from '@/lib/utils'

// shadcn/ui Card (MIT), disesuaikan ke token kepadatan JWM. Hanya bagian
// yang dipakai; CardTitle berupa <h2> agar struktur judul halaman tetap.
function Card({ className, ...props }: ComponentProps<'section'>) {
  return (
    <section
      data-slot="card"
      className={cn('flex flex-col rounded border border-border bg-card text-card-foreground', className)}
      {...props}
    />
  )
}

function CardHeader({ className, ...props }: ComponentProps<'header'>) {
  return (
    <header
      data-slot="card-header"
      className={cn('px-d row-h flex items-center justify-between border-b border-border', className)}
      {...props}
    />
  )
}

function CardTitle({ className, ...props }: ComponentProps<'h2'>) {
  return <h2 data-slot="card-title" className={cn('font-semibold', className)} {...props} />
}

function CardContent({ className, ...props }: ComponentProps<'div'>) {
  return <div data-slot="card-content" className={cn('px-d py-d', className)} {...props} />
}

export { Card, CardContent, CardHeader, CardTitle }

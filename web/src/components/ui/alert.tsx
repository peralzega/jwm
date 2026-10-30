import { cva, type VariantProps } from 'class-variance-authority'
import type { ComponentProps } from 'react'

import { cn } from '@/lib/utils'

// shadcn/ui Alert (MIT), disesuaikan ke token dan palet JWM. role bawaan
// "alert"; pemanggil boleh menggantinya (mis. "status" untuk pesan sukses).
const alertVariants = cva('px-d py-d relative w-full rounded border text-left', {
  variants: {
    variant: {
      default: 'border-border bg-ok-soft text-foreground',
      destructive: 'border-destructive/30 bg-danger-soft text-destructive',
    },
  },
  defaultVariants: { variant: 'default' },
})

function Alert({ className, variant, ...props }: ComponentProps<'div'> & VariantProps<typeof alertVariants>) {
  return <div data-slot="alert" role="alert" className={cn(alertVariants({ variant }), className)} {...props} />
}

export { Alert }

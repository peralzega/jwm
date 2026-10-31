import type { ComponentProps } from 'react'

import { cn } from '@/lib/utils'

// shadcn/ui Input (MIT), disesuaikan ke token kepadatan JWM.
function Input({ className, type, ...props }: ComponentProps<'input'>) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        'row-h px-d w-full min-w-0 rounded border border-input bg-card placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive focus-visible:outline-(length:--focus-ring-w) focus-visible:outline-ring',
        className,
      )}
      {...props}
    />
  )
}

export { Input }

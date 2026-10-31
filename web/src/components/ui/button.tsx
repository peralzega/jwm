import { Slot } from '@radix-ui/react-slot'
import { cva, type VariantProps } from 'class-variance-authority'
import type { ComponentProps } from 'react'

import { cn } from '@/lib/utils'

// shadcn/ui Button (MIT), disesuaikan ke token kepadatan JWM.
const buttonVariants = cva(
  'row-h px-d inline-flex shrink-0 items-center justify-center gap-(--space-sm) rounded border font-medium whitespace-nowrap select-none disabled:pointer-events-none disabled:opacity-50 focus-visible:outline-(length:--focus-ring-w) focus-visible:outline-offset-(--line-w) focus-visible:outline-ring [&_svg]:size-(--icon-size) [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default: 'border-primary bg-primary text-primary-foreground hover:bg-primary-strong',
        outline: 'border-border bg-card text-foreground hover:bg-muted',
        destructive: 'border-border bg-card text-destructive hover:bg-danger-soft',
      },
    },
    defaultVariants: { variant: 'outline' },
  },
)

function Button({
  className,
  variant,
  asChild = false,
  type = 'button',
  ...props
}: ComponentProps<'button'> & VariantProps<typeof buttonVariants> & { asChild?: boolean }) {
  const Comp = asChild ? Slot : 'button'
  return (
    <Comp
      data-slot="button"
      data-variant={variant ?? 'outline'}
      type={asChild ? undefined : type}
      className={cn(buttonVariants({ variant }), className)}
      {...props}
    />
  )
}

export { Button, buttonVariants }

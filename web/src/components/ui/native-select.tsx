import type { ComponentProps } from 'react'

import { cn } from '@/lib/utils'

// shadcn/ui NativeSelect (MIT), disesuaikan ke token kepadatan JWM. Tetap
// <select> bawaan peramban: aman untuk CSP (tanpa <style> sisipan seperti
// Select Radix) dan memakai pemilih bawaan di ponsel. Ikon panah memakai SVG
// sebaris, bukan lucide-react.
function NativeSelect({ className, wrapperClassName, ...props }: ComponentProps<'select'> & { wrapperClassName?: string }) {
  return (
    <div data-slot="native-select-wrapper" className={cn('relative w-full has-[select:disabled]:opacity-50', wrapperClassName)}>
      <select
        data-slot="native-select"
        className={cn(
          'row-h w-full pl-(--density-pad-x) min-w-0 appearance-none rounded border border-input bg-card pr-[calc(var(--density-pad-x)*2_+_var(--icon-size))] disabled:cursor-not-allowed aria-invalid:border-destructive focus-visible:outline-(length:--focus-ring-w) focus-visible:outline-ring',
          className,
        )}
        {...props}
      />
      <svg
        aria-hidden="true"
        data-slot="native-select-icon"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        className="pointer-events-none absolute top-1/2 right-(--density-pad-x) size-(--icon-size) -translate-y-1/2 text-muted-foreground"
      >
        <path d="m6 9 6 6 6-6" />
      </svg>
    </div>
  )
}

export { NativeSelect }

import * as PopoverPrimitive from '@radix-ui/react-popover'
import type { ComponentProps } from 'react'

import { cn } from '@/lib/utils'

// shadcn/ui Popover (MIT), disesuaikan ke token kepadatan JWM. Selalu
// non-modal: mode modal Radix memakai react-remove-scroll yang menyisipkan
// <style> dan melanggar CSP style-src 'self'.
function Popover(props: Omit<ComponentProps<typeof PopoverPrimitive.Root>, 'modal'>) {
  return <PopoverPrimitive.Root data-slot="popover" modal={false} {...props} />
}

function PopoverTrigger(props: ComponentProps<typeof PopoverPrimitive.Trigger>) {
  return <PopoverPrimitive.Trigger data-slot="popover-trigger" {...props} />
}

function PopoverAnchor(props: ComponentProps<typeof PopoverPrimitive.Anchor>) {
  return <PopoverPrimitive.Anchor data-slot="popover-anchor" {...props} />
}

function PopoverContent({
  className,
  align = 'start',
  sideOffset = 2,
  ...props
}: ComponentProps<typeof PopoverPrimitive.Content>) {
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Content
        data-slot="popover-content"
        align={align}
        sideOffset={sideOffset}
        className={cn(
          'z-50 min-w-(--panel-min-w) rounded border border-border bg-popover text-popover-foreground shadow-lg outline-none data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0',
          className,
        )}
        {...props}
      />
    </PopoverPrimitive.Portal>
  )
}

export { Popover, PopoverAnchor, PopoverContent, PopoverTrigger }

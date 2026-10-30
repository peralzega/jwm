import * as TabsPrimitive from '@radix-ui/react-tabs'
import type { ComponentProps } from 'react'

import { cn } from '@/lib/utils'

// shadcn/ui Tabs (MIT), varian garis bawah, disesuaikan ke token JWM.
function Tabs({ className, ...props }: ComponentProps<typeof TabsPrimitive.Root>) {
  return <TabsPrimitive.Root data-slot="tabs" className={cn('gap-d flex flex-col', className)} {...props} />
}

function TabsList({ className, ...props }: ComponentProps<typeof TabsPrimitive.List>) {
  return <TabsPrimitive.List data-slot="tabs-list" className={cn('flex border-b border-border', className)} {...props} />
}

function TabsTrigger({ className, ...props }: ComponentProps<typeof TabsPrimitive.Trigger>) {
  return (
    <TabsPrimitive.Trigger
      data-slot="tabs-trigger"
      className={cn(
        'px-d row-h -mb-(--line-w) border-b-(length:--tab-line-w) border-transparent text-muted-foreground outline-none hover:text-foreground focus-visible:ring-(length:--focus-ring-w) focus-visible:ring-primary disabled:pointer-events-none disabled:opacity-50 data-[state=active]:border-primary data-[state=active]:font-semibold data-[state=active]:text-foreground',
        className,
      )}
      {...props}
    />
  )
}

function TabsContent({ className, ...props }: ComponentProps<typeof TabsPrimitive.Content>) {
  return <TabsPrimitive.Content data-slot="tabs-content" className={cn('outline-none', className)} {...props} />
}

export { Tabs, TabsContent, TabsList, TabsTrigger }

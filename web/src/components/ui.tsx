// Komponen dasar aplikasi: lapisan tipis di atas shadcn/ui (src/components/ui)
// dengan API lama, agar halaman tidak perlu diubah saat adopsi bertahap.

import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode, SelectHTMLAttributes } from 'react'
import { useId } from 'react'

import { Alert as UiAlert } from './ui/alert'
import { Button as UiButton } from './ui/button'
import { Card, CardContent, CardHeader, CardTitle } from './ui/card'
import { Input } from './ui/input'
import { NativeSelect } from './ui/native-select'

type Variant = 'primary' | 'secondary' | 'danger'

const VARIANT = { primary: 'default', secondary: 'outline', danger: 'destructive' } as const

export function Button({ variant = 'secondary', ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant }) {
  return <UiButton variant={VARIANT[variant]} {...props} />
}

export function TextField({
  label,
  hint,
  ...props
}: InputHTMLAttributes<HTMLInputElement> & { label: string; hint?: string }) {
  const id = useId()
  return (
    <div className="flex flex-col gap-(--space-sm)">
      <label htmlFor={id} className="font-medium">
        {label}
      </label>
      <Input id={id} aria-describedby={hint ? `${id}-hint` : undefined} {...props} />
      {hint && (
        <span id={`${id}-hint`} className="text-muted-foreground">
          {hint}
        </span>
      )}
    </div>
  )
}

export function SelectField({
  label,
  children,
  ...props
}: SelectHTMLAttributes<HTMLSelectElement> & { label: string; children: ReactNode }) {
  const id = useId()
  return (
    <div className="flex flex-col gap-(--space-sm)">
      <label htmlFor={id} className="font-medium">
        {label}
      </label>
      <NativeSelect id={id} {...props}>
        {children}
      </NativeSelect>
    </div>
  )
}

export function Alert({ kind = 'error', children }: { kind?: 'error' | 'ok'; children: ReactNode }) {
  return (
    <UiAlert variant={kind === 'error' ? 'destructive' : 'default'} role={kind === 'error' ? 'alert' : 'status'}>
      {children}
    </UiAlert>
  )
}

export function Panel({ title, actions, children }: { title: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <Card aria-label={title}>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {actions}
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  )
}

export function Loading() {
  return (
    <p role="status" className="text-muted-foreground px-d py-d">
      Memuat…
    </p>
  )
}

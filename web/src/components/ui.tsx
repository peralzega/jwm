import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode, SelectHTMLAttributes } from 'react'
import { useId } from 'react'

type Variant = 'primary' | 'secondary' | 'danger'

const VARIANT: Record<Variant, string> = {
  primary: 'bg-accent text-white hover:bg-accent-strong border-accent',
  secondary: 'bg-surface text-ink hover:bg-canvas border-line',
  danger: 'bg-surface text-danger hover:bg-danger-soft border-line',
}

export function Button({
  variant = 'secondary',
  className = '',
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant }) {
  return (
    <button
      type="button"
      className={`row-h px-d inline-flex items-center rounded border font-medium disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent ${VARIANT[variant]} ${className}`}
      {...props}
    />
  )
}

const FIELD = 'row-h px-d w-full rounded border border-line bg-surface focus-visible:outline-2 focus-visible:outline-accent'

export function TextField({
  label,
  hint,
  ...props
}: InputHTMLAttributes<HTMLInputElement> & { label: string; hint?: string }) {
  const id = useId()
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="font-medium">
        {label}
      </label>
      <input id={id} className={FIELD} aria-describedby={hint ? `${id}-hint` : undefined} {...props} />
      {hint && (
        <span id={`${id}-hint`} className="text-muted">
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
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="font-medium">
        {label}
      </label>
      <select id={id} className={FIELD} {...props}>
        {children}
      </select>
    </div>
  )
}

export function Alert({ kind = 'error', children }: { kind?: 'error' | 'ok'; children: ReactNode }) {
  const cls = kind === 'error' ? 'bg-danger-soft text-danger border-danger/30' : 'bg-ok-soft text-ink border-line'
  return (
    <div role={kind === 'error' ? 'alert' : 'status'} className={`px-d py-d rounded border ${cls}`}>
      {children}
    </div>
  )
}

export function Panel({ title, actions, children }: { title: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <section className="rounded border border-line bg-surface" aria-label={title}>
      <header className="px-d row-h flex items-center justify-between border-b border-line">
        <h2 className="font-semibold">{title}</h2>
        {actions}
      </header>
      <div className="px-d py-d">{children}</div>
    </section>
  )
}

export function Loading() {
  return (
    <p role="status" className="text-muted px-d py-d">
      Memuat…
    </p>
  )
}

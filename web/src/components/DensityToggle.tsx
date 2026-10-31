import { errorMessage } from '../api/client'
import { useSetDensity } from '../auth/session'
import { useDensity, type Density } from '../density'
import { Tooltip, TooltipContent, TooltipTrigger } from './ui/tooltip'

const OPTIONS: { value: Density; label: string; hint: string }[] = [
  { value: 'compact', label: 'Ringkas', hint: 'Tampilan rapat: baris tabel 32 px, huruf 13 px.' },
  { value: 'normal', label: 'Normal', hint: 'Tampilan lega: baris tabel 40 px, huruf 14 px.' },
]

/** Pilihan kepadatan tampilan, disimpan sebagai preferensi pengguna di server. */
export function DensityToggle() {
  const density = useDensity()
  const set = useSetDensity()

  return (
    <div role="group" aria-label="Kepadatan tampilan" className="flex items-center">
      {OPTIONS.map((o, i) => (
        <Tooltip key={o.value}>
          <TooltipTrigger asChild>
            <button
              type="button"
              aria-pressed={density === o.value}
              disabled={set.isPending}
              onClick={() => {
                if (density !== o.value) set.mutate(o.value)
              }}
              className={`row-h px-d border border-line focus-visible:outline-(length:--focus-ring-w) focus-visible:outline-primary ${
                i === 0 ? 'rounded-l' : '-ml-(--line-w) rounded-r'
              } ${density === o.value ? 'bg-canvas font-semibold' : 'bg-surface hover:bg-canvas'}`}
            >
              {o.label}
            </button>
          </TooltipTrigger>
          <TooltipContent>{o.hint}</TooltipContent>
        </Tooltip>
      ))}
      {set.isError && (
        <span role="alert" className="px-d text-danger">
          {errorMessage(set.error)}
        </span>
      )}
    </div>
  )
}

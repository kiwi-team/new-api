/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useEffect, useState, type KeyboardEvent } from 'react'

import { Input } from '@/components/ui/input'

import { round6 } from '../lib'

type DualPriceInputProps = {
  /** Price in USD; the CNY field is derived from it via `rate`. */
  value: number | null
  rate: number
  disabled?: boolean
  onCommit: (usd: number) => void
}

/**
 * Inline price cell in both currencies: editing either field converts to the
 * other, and blur or Enter commits the USD value. Commits are skipped when the
 * rounded value is unchanged so tabbing through the table saves nothing.
 */
export function DualPriceInput({
  value,
  rate,
  disabled,
  onCommit,
}: DualPriceInputProps) {
  const [usdText, setUsdText] = useState('')
  const [cnyText, setCnyText] = useState('')

  useEffect(() => {
    if (value === null || value === undefined) {
      setUsdText('')
      setCnyText('')
      return
    }
    setUsdText(String(value))
    setCnyText(String(round6(value * rate)))
  }, [value, rate])

  if (disabled) {
    return <span className='text-muted-foreground'>-</span>
  }

  const commitFromUsd = () => {
    const parsed = usdText === '' ? 0 : Number.parseFloat(usdText)
    const usd = Number.isNaN(parsed) ? 0 : parsed
    setCnyText(String(round6(usd * rate)))
    if (round6(usd) !== round6(value ?? 0)) onCommit(round6(usd))
  }

  const commitFromCny = () => {
    const parsed = cnyText === '' ? 0 : Number.parseFloat(cnyText)
    const usd = (Number.isNaN(parsed) ? 0 : parsed) / rate
    setUsdText(String(round6(usd)))
    if (round6(usd) !== round6(value ?? 0)) onCommit(round6(usd))
  }

  const commitOnEnter =
    (commit: () => void) => (event: KeyboardEvent<HTMLInputElement>) => {
      if (event.key === 'Enter') commit()
    }

  return (
    <div className='flex w-40 flex-col gap-1'>
      <div className='relative'>
        <span className='text-muted-foreground pointer-events-none absolute inset-y-0 start-2 flex items-center text-xs'>
          $
        </span>
        <Input
          className='h-8 ps-5 text-xs'
          value={usdText}
          onChange={(event) => setUsdText(event.target.value)}
          onBlur={commitFromUsd}
          onKeyDown={commitOnEnter(commitFromUsd)}
          inputMode='decimal'
        />
      </div>
      <div className='relative'>
        <span className='text-muted-foreground pointer-events-none absolute inset-y-0 start-2 flex items-center text-xs'>
          ¥
        </span>
        <Input
          className='h-8 ps-5 text-xs'
          value={cnyText}
          onChange={(event) => setCnyText(event.target.value)}
          onBlur={commitFromCny}
          onKeyDown={commitOnEnter(commitFromCny)}
          inputMode='decimal'
        />
      </div>
    </div>
  )
}

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
import { useEffect, useRef, useState, type KeyboardEvent } from 'react'

import { Input } from '@/components/ui/input'

import { formatPrice, nearlyEqual, round6 } from '../lib'

type DualPriceInputProps = {
  /** Price in USD; the CNY field is derived from it via `rate`. */
  value: number | null
  rate: number
  disabled?: boolean
  onCommit: (usd: number) => void
}

/**
 * Inline price cell in both currencies: editing either field converts to the
 * other, and blur or Enter commits the USD value.
 *
 * Only USD is stored, so CNY is a derived view and the round trip through
 * `usd * rate` always leaves noise in the last digits. What is shown is what
 * gets saved: both fields render through `formatPrice`, including while
 * focused, so clicking a cell never swaps ¥1.50 for its raw ¥1.499998.
 *
 * Two guards keep that noise out of the database: a field that was not actually
 * typed into commits nothing on blur, and a commit landing on the same price
 * (`nearlyEqual`) is skipped — otherwise retyping the same CNY amount writes a
 * value that reads back unchanged, which looks exactly like the edit never
 * reached the database.
 */
export function DualPriceInput({
  value,
  rate,
  disabled,
  onCommit,
}: DualPriceInputProps) {
  const [usdText, setUsdText] = useState('')
  const [cnyText, setCnyText] = useState('')
  // Focus and blur alone must not count as an edit.
  const usdEdited = useRef(false)
  const cnyEdited = useRef(false)

  useEffect(() => {
    if (value === null || value === undefined) {
      setUsdText('')
      setCnyText('')
    } else {
      setUsdText(formatPrice(round6(value)))
      setCnyText(formatPrice(round6(value * rate)))
    }
    usdEdited.current = false
    cnyEdited.current = false
  }, [value, rate])

  if (disabled) {
    return <span className='text-muted-foreground'>-</span>
  }

  const commit = (usd: number) => {
    usdEdited.current = false
    cnyEdited.current = false
    setUsdText(formatPrice(usd))
    setCnyText(formatPrice(round6(usd * rate)))
    if (!nearlyEqual(usd, value ?? 0)) onCommit(usd)
  }

  const commitFromUsd = () => {
    const parsed = usdText === '' ? 0 : Number.parseFloat(usdText)
    commit(round6(Number.isNaN(parsed) ? 0 : parsed))
  }

  const commitFromCny = () => {
    const parsed = cnyText === '' ? 0 : Number.parseFloat(cnyText)
    commit(round6((Number.isNaN(parsed) ? 0 : parsed) / rate))
  }

  const commitOnEnter =
    (commitField: () => void) => (event: KeyboardEvent<HTMLInputElement>) => {
      if (event.key === 'Enter') commitField()
    }

  const shownUsd = value === null ? '' : formatPrice(round6(value))
  const shownCny = value === null ? '' : formatPrice(round6(value * rate))

  return (
    <div className='flex w-40 flex-col gap-1'>
      <div className='relative'>
        <span className='text-muted-foreground pointer-events-none absolute inset-y-0 start-2 flex items-center text-xs'>
          $
        </span>
        <Input
          className='h-8 ps-5 text-xs'
          value={usdText}
          onChange={(event) => {
            usdEdited.current = true
            setUsdText(event.target.value)
          }}
          onBlur={() => {
            if (usdEdited.current) commitFromUsd()
            else setUsdText(shownUsd)
          }}
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
          onChange={(event) => {
            cnyEdited.current = true
            setCnyText(event.target.value)
          }}
          onBlur={() => {
            if (cnyEdited.current) commitFromCny()
            else setCnyText(shownCny)
          }}
          onKeyDown={commitOnEnter(commitFromCny)}
          inputMode='decimal'
        />
      </div>
    </div>
  )
}

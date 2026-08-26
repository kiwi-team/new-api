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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, test } from 'vitest'

import { LogFilterCombobox } from '../log-filter-combobox'

const options = [
  { value: 'alpha-key', label: 'Alpha Key' },
  { value: 'beta-key', label: 'Beta Key' },
]

function Harness(props: {
  initialValue?: string
  type?: React.HTMLInputTypeAttribute
}) {
  const [value, setValue] = useState(props.initialValue ?? '')

  return (
    <>
      <LogFilterCombobox
        options={options}
        value={value}
        onValueChange={setValue}
        placeholder='Token Name'
        type={props.type}
      />
      <output data-testid='value'>{value}</output>
    </>
  )
}

describe('usage log filter combobox', () => {
  test('filters options by a case-insensitive partial match and selects one', async () => {
    render(<Harness />)
    const input = screen.getByRole('combobox', { name: 'Token Name' })

    fireEvent.focus(input)
    fireEvent.change(input, { target: { value: 'ALP' } })

    expect(screen.getByRole('option', { name: 'Alpha Key' })).toBeVisible()
    expect(screen.queryByRole('option', { name: 'Beta Key' })).toBeNull()

    fireEvent.mouseDown(screen.getByRole('option', { name: 'Alpha Key' }))
    expect(screen.getByTestId('value')).toHaveTextContent('alpha-key')
    await waitFor(() => {
      expect(input).toHaveValue('Alpha Key')
      expect(input).toHaveAttribute('aria-expanded', 'false')
    })
  })

  test('keeps a custom value when no fetched option matches', () => {
    render(<Harness />)
    const input = screen.getByRole('combobox', { name: 'Token Name' })

    fireEvent.focus(input)
    fireEvent.change(input, { target: { value: 'manual-key' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(screen.getByTestId('value')).toHaveTextContent('manual-key')
    expect(input).toHaveValue('manual-key')
  })

  test('clears a selected filter with the clear button', () => {
    render(<Harness initialValue='alpha-key' />)
    const input = screen.getByRole('combobox', { name: 'Token Name' })
    const clearButton = screen.getByRole('button', { name: 'Clear' })

    expect(input).toHaveValue('Alpha Key')
    fireEvent.mouseDown(clearButton)
    fireEvent.click(clearButton)

    expect(screen.getByTestId('value')).toBeEmptyDOMElement()
    expect(input).toHaveValue('')
    expect(screen.queryByRole('button', { name: 'Clear' })).toBeNull()
  })

  test('masks a sensitive selected value while retaining combobox semantics', () => {
    render(<Harness type='password' />)

    const input = screen.getByRole('combobox', { name: 'Token Name' })
    expect(input).toHaveAttribute('type', 'password')
    expect(input).toHaveAttribute('aria-autocomplete', 'list')
  })
})

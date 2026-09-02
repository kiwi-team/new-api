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
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { beforeAll, describe, expect, test } from 'vitest'

import { RetryChainDisplay } from '../retry-chain-display'

describe('retry chain display', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      'Retry Chain': 'Retry Chain',
    })
  })

  test('shows the complete retry chain without requiring interaction', () => {
    render(<RetryChainDisplay chain='1578(343ms) → 1577' />)

    expect(screen.getByText('1578(343ms) → 1577')).toBeVisible()
    expect(
      screen.getByLabelText('Retry Chain: 1578(343ms) → 1577')
    ).toBeVisible()
  })
})

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
import { describe, expect, it } from 'vitest'

import { multiSelectOptionMatchesQuery } from '../search'

describe('multiSelectOptionMatchesQuery', () => {
  it('matches a channel option by ID keyword', () => {
    expect(
      multiSelectOptionMatchesQuery(
        '1577',
        'Primary DeepSeek (ID: 1577)',
        '577'
      )
    ).toBe(true)
  })

  it('matches a channel option by name keyword without case sensitivity', () => {
    expect(
      multiSelectOptionMatchesQuery(
        '1577',
        'Primary DeepSeek (ID: 1577)',
        'deepseek'
      )
    ).toBe(true)
  })

  it('rejects keywords absent from both the ID and name', () => {
    expect(
      multiSelectOptionMatchesQuery(
        '1577',
        'Primary DeepSeek (ID: 1577)',
        'claude'
      )
    ).toBe(false)
  })
})

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

import type { UsageLog } from '../../data/schema'
import { formatModelName } from '../format'

const mappedLog = {
  model_name: 'deepseek-v4-flash',
  other: JSON.stringify({
    is_model_mapped: true,
    upstream_model_name: 'deepseek-v4-flash-ga-260731',
  }),
} as UsageLog

describe('formatModelName', () => {
  it('does not expose the actual model when the viewer is not root', () => {
    expect(formatModelName(mappedLog, false)).toEqual({
      name: 'deepseek-v4-flash',
      isMapped: false,
      actualModel: undefined,
    })
  })

  it('exposes the actual model when the viewer is root', () => {
    expect(formatModelName(mappedLog, true)).toEqual({
      name: 'deepseek-v4-flash',
      isMapped: true,
      actualModel: 'deepseek-v4-flash-ga-260731',
    })
  })
})

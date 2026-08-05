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
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  getDefaultUsageDateRange,
  usageDateRangeTimestamps,
  usageRowKey,
} from '../lib'
import type { ModelUsageRow } from '../types'

describe('model usage date boundaries', () => {
  it('uses seven complete calendar days in UTC+8 by default', () => {
    const range = getDefaultUsageDateRange(new Date('2026-08-04T01:30:00.000Z'))

    assert.deepEqual(range, { start: '2026-07-29', end: '2026-08-04' })
  })

  it('converts selected dates to inclusive UTC+8 day boundaries', () => {
    const range = usageDateRangeTimestamps('2026-08-01', '2026-08-02')

    assert.equal(
      range.start_timestamp,
      Date.parse('2026-08-01T00:00:00+08:00') / 1000
    )
    assert.equal(
      range.end_timestamp,
      Date.parse('2026-08-02T23:59:59+08:00') / 1000
    )
  })

  it('keeps different users distinct in aggregate row keys', () => {
    const base = { date: '2026-08-01', token_id: 7, model_name: 'model' }
    assert.notEqual(
      usageRowKey({ ...base, user_id: 1 } as ModelUsageRow),
      usageRowKey({ ...base, user_id: 2 } as ModelUsageRow)
    )
  })
})

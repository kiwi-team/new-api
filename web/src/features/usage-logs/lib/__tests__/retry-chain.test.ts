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
import { describe, test } from 'node:test'

import { formatRetryChain } from '../format'

describe('formatRetryChain', () => {
  test('annotates each attempt with its elapsed time', () => {
    assert.equal(
      formatRetryChain([3, 16], [1200, 340]),
      '3(1.20s) → 16(340ms)'
    )
  })

  test('switches to seconds at one second', () => {
    assert.equal(formatRetryChain([1], [999]), '1(999ms)')
    assert.equal(formatRetryChain([1], [1000]), '1(1.00s)')
  })

  test('falls back to bare ids when timings were never recorded', () => {
    assert.equal(formatRetryChain([3, 16], undefined), '3 → 16')
    assert.equal(formatRetryChain([3, 16], []), '3 → 16')
  })

  test('degrades per entry when a timing is missing or unusable', () => {
    // A short array happens when an attempt dies before its duration is
    // captured; zero and negative values are not meaningful durations.
    assert.equal(formatRetryChain([3, 16, 7], [1200]), '3(1.20s) → 16 → 7')
    assert.equal(formatRetryChain([3, 16], [1200, 0]), '3(1.20s) → 16')
    assert.equal(formatRetryChain([3, 16], [1200, -5]), '3(1.20s) → 16')
  })

  test('returns undefined when there is no chain to show', () => {
    assert.equal(formatRetryChain(undefined, undefined), undefined)
    assert.equal(formatRetryChain([], [100]), undefined)
  })
})

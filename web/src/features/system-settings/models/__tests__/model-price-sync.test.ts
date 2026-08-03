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

import { collectPricedModels } from '../model-price-sync'

describe('collectPricedModels', () => {
  test('merges and sorts model names across the three pricing blobs', () => {
    assert.deepEqual(
      collectPricedModels({
        ModelPrice: '{"gpt-4o":0.01}',
        ModelRatio: '{"claude-sonnet-4-5":2}',
        CompletionRatio: '{"gpt-4o":3,"gemini-3-pro":2}',
      }),
      ['claude-sonnet-4-5', 'gemini-3-pro', 'gpt-4o']
    )
  })

  test('ignores blobs that are missing, empty, or malformed', () => {
    // A single bad blob must not hide the models from the good ones.
    assert.deepEqual(
      collectPricedModels({
        ModelPrice: 'not json',
        ModelRatio: '',
        CompletionRatio: '{"gpt-4o":3}',
      }),
      ['gpt-4o']
    )
    assert.deepEqual(collectPricedModels({}), [])
  })

  test('ignores JSON that is not an object of model names', () => {
    assert.deepEqual(
      collectPricedModels({ ModelPrice: '["gpt-4o"]', ModelRatio: 'null' }),
      []
    )
  })
})

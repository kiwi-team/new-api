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

import {
  parseSpecialChannelRules,
  serializeSpecialChannelRules,
} from '../special-channels'

describe('parseSpecialChannelRules', () => {
  test('reads model name, tiers, and selection mode', () => {
    const rules = parseSpecialChannelRules(
      '[{"model_name":"^gemini-","channel_ids":[[3,4],[7]],"random_type":"random"}]'
    )

    assert.equal(rules.length, 1)
    assert.equal(rules[0].modelName, '^gemini-')
    assert.equal(rules[0].randomType, 'random')
    assert.deepEqual(
      rules[0].tiers.map((tier) => tier.ids),
      [[3, 4], [7]]
    )
  })

  test('defaults selection mode to order', () => {
    const rules = parseSpecialChannelRules(
      '[{"model_name":"gpt-4o","channel_ids":[[1]]}]'
    )

    assert.equal(rules[0].randomType, 'order')
  })

  test('returns no rules for empty, malformed, or non-array input', () => {
    assert.deepEqual(parseSpecialChannelRules(''), [])
    assert.deepEqual(parseSpecialChannelRules(null), [])
    assert.deepEqual(parseSpecialChannelRules('not json'), [])
    assert.deepEqual(parseSpecialChannelRules('{"a":1}'), [])
  })

  test('drops rules with no model name and tiers with no channel', () => {
    const rules = parseSpecialChannelRules(
      '[{"model_name":"  ","channel_ids":[[1]]},{"model_name":"gpt-4o","channel_ids":[[],[0],[9]]}]'
    )

    assert.equal(rules.length, 1)
    assert.deepEqual(
      rules[0].tiers.map((tier) => tier.ids),
      [[9]]
    )
  })
})

describe('serializeSpecialChannelRules', () => {
  test('round-trips a rule unchanged', () => {
    const original =
      '[{"model_name":"^gemini-","channel_ids":[[3,4],[7]],"random_type":"order"}]'

    const restored = serializeSpecialChannelRules(
      parseSpecialChannelRules(original)
    )

    assert.deepEqual(JSON.parse(restored), JSON.parse(original))
  })

  test('preserves tier order, which decides fallback priority', () => {
    const json = serializeSpecialChannelRules([
      {
        uid: 'r1',
        modelName: 'gpt-4o',
        randomType: 'order',
        tiers: [
          { uid: 't1', ids: [5] },
          { uid: 't2', ids: [1, 2] },
          { uid: 't3', ids: [9] },
        ],
      },
    ])

    assert.deepEqual(JSON.parse(json)[0].channel_ids, [[5], [1, 2], [9]])
  })

  test('drops rules that could never match', () => {
    const json = serializeSpecialChannelRules([
      {
        uid: 'a',
        modelName: '  ',
        randomType: 'order',
        tiers: [{ uid: 't1', ids: [1] }],
      },
      {
        uid: 'b',
        modelName: 'gpt-4o',
        randomType: 'order',
        tiers: [{ uid: 't2', ids: [] }],
      },
      {
        uid: 'c',
        modelName: 'claude',
        randomType: 'order',
        tiers: [{ uid: 't3', ids: [3] }],
      },
    ])

    assert.deepEqual(JSON.parse(json), [
      { model_name: 'claude', channel_ids: [[3]], random_type: 'order' },
    ])
  })

  test('serializes an empty list as an empty array, not an empty string', () => {
    // The option value is always valid JSON so the backend's Unmarshal of a
    // cleared key yields an empty slice rather than a parse error.
    assert.equal(serializeSpecialChannelRules([]), '[]')
  })
})

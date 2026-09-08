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
  parseChannelRules,
  reorderTiers,
  serializeChannelRules,
} from '../channel-rules'

describe('parseChannelRules', () => {
  test('reads both tier shapes the backend writes', () => {
    const rules = parseChannelRules(
      '{"gpt-4o":{"retry":2,"random_type":"random","disable_channels":[10,23],"channels":[{"ids":[3,4]},{"id":5}]}}'
    )

    assert.equal(rules.length, 1)
    assert.equal(rules[0].modelKey, 'gpt-4o')
    assert.equal(rules[0].retry, 2)
    assert.equal(rules[0].randomType, 'random')
    assert.deepEqual(rules[0].disableChannels, [10, 23])
    assert.deepEqual(
      rules[0].tiers.map((tier) => tier.ids),
      [[3, 4], [5]]
    )
  })

  test('defaults missing fields instead of producing NaN or undefined', () => {
    const rules = parseChannelRules('{"claude":{}}')

    const { uid, ...rule } = rules[0]
    assert.equal(typeof uid, 'string')
    assert.deepEqual(rule, {
      modelKey: 'claude',
      retry: 0,
      randomType: 'order',
      raceTimeout: 25,
      disableChannels: [],
      tiers: [],
    })
  })

  test('returns no rules for empty, malformed, or non-object input', () => {
    assert.deepEqual(parseChannelRules(''), [])
    assert.deepEqual(parseChannelRules(null), [])
    assert.deepEqual(parseChannelRules('not json'), [])
    assert.deepEqual(parseChannelRules('[1,2]'), [])
  })

  test('drops tiers that reference no channel', () => {
    const rules = parseChannelRules(
      '{"gpt-4o":{"channels":[{"ids":[]},{"id":0},{"ids":[7]}]}}'
    )

    assert.deepEqual(
      rules[0].tiers.map((tier) => tier.ids),
      [[7]]
    )
  })
})

describe('serializeChannelRules', () => {
  test('round-trips group_ratio, name, and weight the editor does not expose', () => {
    const original =
      '{"gpt-4o":{"retry":1,"random_type":"order","disable_channels":[],"channels":[{"id":23,"group_ratio":{"default":1,"zero":0},"weight":10},{"ids":[24,25],"name":"^azure-"}]}}'

    const restored = serializeChannelRules(parseChannelRules(original))

    assert.deepEqual(JSON.parse(restored), JSON.parse(original))
  })

  test('returns an empty string when nothing is configured', () => {
    assert.equal(serializeChannelRules([]), '')
    assert.equal(
      serializeChannelRules([
        {
          uid: 'x',
          modelKey: '   ',
          retry: 0,
          randomType: 'order',
          raceTimeout: 25,
          disableChannels: [],
          tiers: [],
        },
      ]),
      ''
    )
  })

  test('drops tiers with no channels but keeps the rule', () => {
    const json = serializeChannelRules([
      {
        uid: 'r1',
        modelKey: 'gpt-4o',
        retry: 0,
        randomType: 'order',
        raceTimeout: 25,
        disableChannels: [],
        tiers: [
          { uid: 't1', ids: [], extra: {} },
          { uid: 't2', ids: [9], extra: {} },
        ],
      },
    ])

    assert.deepEqual(JSON.parse(json)['gpt-4o'].channels, [{ ids: [9] }])
  })

  test('preserves tier order, which decides relay priority', () => {
    const json = serializeChannelRules([
      {
        uid: 'r1',
        modelKey: 'gpt-4o',
        retry: 0,
        randomType: 'order',
        raceTimeout: 25,
        disableChannels: [],
        tiers: [
          { uid: 't1', ids: [3], extra: {} },
          { uid: 't2', ids: [1], extra: {} },
          { uid: 't3', ids: [2], extra: {} },
        ],
      },
    ])

    assert.deepEqual(JSON.parse(json)['gpt-4o'].channels, [
      { ids: [3] },
      { ids: [1] },
      { ids: [2] },
    ])
  })
})

describe('race channel rules', () => {
  test('round-trips race mode and its launch delay', () => {
    const original =
      '{"gpt-4o":{"retry":9,"random_type":"race","race_timeout":17,"disable_channels":[],"channels":[{"ids":[1,2]},{"ids":[3]}]}}'

    const rules = parseChannelRules(original)
    assert.equal(rules[0].randomType, 'race')
    assert.equal(rules[0].raceTimeout, 17)
    assert.deepEqual(JSON.parse(serializeChannelRules(rules)), JSON.parse(original))
  })

  test('uses the 25 second default for a missing race timeout', () => {
    const rules = parseChannelRules(
      '{"gpt-4o":{"random_type":"race","channels":[{"ids":[1]}]}}'
    )

    assert.equal(rules[0].raceTimeout, 25)
  })
})

describe('reorderTiers', () => {
  const tiers = [
    { uid: 't1', ids: [1], extra: {} },
    { uid: 't2', ids: [2], extra: {} },
    { uid: 't3', ids: [3], extra: {} },
  ]

  test('moves a tier to the target index', () => {
    assert.deepEqual(
      reorderTiers(tiers, 0, 2).map((tier) => tier.ids),
      [[2], [3], [1]]
    )
  })

  test('leaves the list untouched for no-op or out-of-range moves', () => {
    assert.equal(reorderTiers(tiers, 1, 1), tiers)
    assert.equal(reorderTiers(tiers, -1, 0), tiers)
    assert.equal(reorderTiers(tiers, 0, 9), tiers)
  })
})

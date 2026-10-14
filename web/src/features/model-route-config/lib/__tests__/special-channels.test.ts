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
import { describe, expect, test } from 'vitest'

import {
  parseGeminiFileRouting,
  parseSpecialChannelRules,
  serializeSpecialChannelRules,
} from '../special-channels'

describe('large file routing configuration', () => {
  test('missing settings are disabled and cleared rules remain an empty array', () => {
    expect(parseGeminiFileRouting()).toEqual({
      enabled: false,
      mode: 'forward',
      chat_model_patterns: [],
      inline_threshold_bytes: 10485760,
      max_inline_request_bytes: 33554432,
    })
    expect(parseSpecialChannelRules(undefined)).toEqual([])
    expect(serializeSpecialChannelRules([])).toBe('[]')
  })

  test('round trips multiple models with tier priority and selection modes', () => {
    const raw =
      '[{"model_name":"^gemini-","channel_ids":[[3,4],[7]],"random_type":"random"},{"model_name":"alias","channel_ids":[[9]],"random_type":"order"}]'
    expect(
      JSON.parse(serializeSpecialChannelRules(parseSpecialChannelRules(raw)))
    ).toEqual(JSON.parse(raw))
  })

  test('an omitted selection mode defaults to order', () => {
    expect(
      parseSpecialChannelRules(
        '[{"model_name":"alias","channel_ids":[[1]]}]'
      )[0].randomType
    ).toBe('order')
  })

  test('accepts an upstream request limit above 256 MiB', () => {
    expect(
      parseGeminiFileRouting('{"max_inline_request_bytes":335544320}')
        .max_inline_request_bytes
    ).toBe(335544320)
  })

  test.each([
    '',
    'null',
    'not json',
    '{}',
    '[{"model_name":" ","channel_ids":[[1]]}]',
    '[{"model_name":"x","channel_ids":[[]]}]',
    '[{"model_name":"x","channel_ids":[[0]]}]',
  ])(
    'rejects malformed saved rules %s instead of silently clearing them',
    (raw) => {
      expect(() => parseSpecialChannelRules(raw)).toThrow()
    }
  )

  test('rejects incomplete drafts instead of dropping them on save', () => {
    expect(() =>
      serializeSpecialChannelRules([
        {
          uid: '1',
          modelName: '',
          randomType: 'order',
          tiers: [{ uid: '2', ids: [1] }],
        },
      ])
    ).toThrow()
  })

  test.each([
    'null',
    '{"mode":"other"}',
    '{"inline_threshold_bytes":0}',
    '{"max_inline_request_bytes":0}',
    '{"max_inline_request_bytes":-1}',
    '{"max_inline_request_bytes":1.5}',
    '{"chat_model_patterns":[""]}',
  ])('rejects invalid file policy %s', (raw) => {
    expect(() => parseGeminiFileRouting(raw)).toThrow()
  })
})

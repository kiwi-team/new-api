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
  CHANNEL_TYPES,
  CHANNEL_TYPE_CODEX,
  CHANNEL_TYPE_NEW_API,
  CHANNEL_TYPE_OPTIONS,
  MODEL_FETCHABLE_TYPES,
} from '../../constants'
import { CHANNEL_TYPE_ADVANCED_CUSTOM } from '../advanced-custom'
import { getChannelTypeConfig } from '../channel-type-config'
import { getChannelTypeIcon, getChannelTypeLabel } from '../channel-utils'

/**
 * Channel type IDs are sent to the backend verbatim — there is no mapping
 * layer. They must therefore match `constant/channel.go` exactly.
 *
 * This fork's numbering diverges from upstream new-api from ID 51 onwards
 * (the fork took 51/52 for Sensenova / Visual VolcEngine before upstream
 * assigned them to Jimeng / Vidu). A frontend imported from upstream carries
 * upstream's numbering, which silently creates channels of the wrong provider:
 * picking "Replicate" would have written a Submodel channel.
 *
 * The expectations below are transcribed from constant/channel.go. If a channel
 * is added or renumbered there, update this table in the same change.
 */
const BACKEND_CHANNEL_IDS = {
  Kling: 50,
  Sensenova: 51,
  VisualVolcEngine: 52,
  Jimeng: 53,
  Serper: 54,
  Vidu: 55,
  Submodel: 56,
  DoubaoVideo: 57,
  Sora: 58,
  ElevenLabs: 59,
  AliDashScope: 60,
  FAL: 61,
  Replicate: 62,
  Pixverse: 63,
  Ltx: 64,
  AwsV2: 65,
  WorldLabs: 66,
  FALSync: 67,
  Codex: 68,
  RunwayML: 69,
  PPIO: 70,
  Hedra: 71,
  HeyGen: 72,
  Reve: 73,
  AdvancedCustom: 74,
  Sub2API: 75,
  NewAPI: 76,
  MiniMaxVideo: 77,
} as const

describe('channel type IDs match the backend', () => {
  test('every backend channel is selectable in the UI', () => {
    for (const [name, id] of Object.entries(BACKEND_CHANNEL_IDS)) {
      assert.ok(
        CHANNEL_TYPES[id as keyof typeof CHANNEL_TYPES],
        `${name} (${id}) is missing from CHANNEL_TYPES, so it cannot be created`
      )
      assert.ok(
        CHANNEL_TYPE_OPTIONS.some((option) => option.value === id),
        `${name} (${id}) is missing from the type picker`
      )
    }
  })

  test('exported single IDs point at the right provider', () => {
    assert.equal(CHANNEL_TYPE_CODEX, BACKEND_CHANNEL_IDS.Codex)
    assert.equal(CHANNEL_TYPE_NEW_API, BACKEND_CHANNEL_IDS.NewAPI)
    assert.equal(
      CHANNEL_TYPE_ADVANCED_CUSTOM,
      BACKEND_CHANNEL_IDS.AdvancedCustom
    )
  })

  // These four carry provider-specific base URLs and key hints; pointing them
  // at the wrong ID pre-fills a different provider's endpoint.
  test('type configs sit on the right IDs', () => {
    assert.equal(
      getChannelTypeConfig(BACKEND_CHANNEL_IDS.Replicate).defaultBaseUrl,
      'https://api.replicate.com'
    )
    assert.equal(
      getChannelTypeConfig(BACKEND_CHANNEL_IDS.NewAPI).icon,
      'NewAPI'
    )
    assert.equal(
      getChannelTypeConfig(BACKEND_CHANNEL_IDS.Sub2API).icon,
      'Sub2API'
    )
  })

  test('icons follow the backend provider, not upstream numbering', () => {
    assert.equal(getChannelTypeIcon(BACKEND_CHANNEL_IDS.Jimeng), 'Jimeng')
    assert.equal(getChannelTypeIcon(BACKEND_CHANNEL_IDS.Vidu), 'Vidu')
    assert.equal(getChannelTypeIcon(BACKEND_CHANNEL_IDS.Replicate), 'Replicate')
    assert.equal(getChannelTypeIcon(BACKEND_CHANNEL_IDS.DoubaoVideo), 'Doubao')
    assert.equal(getChannelTypeIcon(BACKEND_CHANNEL_IDS.NewAPI), 'NewAPI')
  })

  test('model discovery is enabled for the OpenAI-compatible aggregators', () => {
    for (const id of [
      BACKEND_CHANNEL_IDS.Codex,
      BACKEND_CHANNEL_IDS.AdvancedCustom,
      BACKEND_CHANNEL_IDS.Sub2API,
      BACKEND_CHANNEL_IDS.NewAPI,
    ]) {
      assert.ok(
        MODEL_FETCHABLE_TYPES.has(id),
        `channel ${id} should support upstream model discovery`
      )
    }
  })

  test('labels do not silently name another provider', () => {
    assert.equal(getChannelTypeLabel(BACKEND_CHANNEL_IDS.Jimeng), 'Jimeng')
    assert.equal(getChannelTypeLabel(BACKEND_CHANNEL_IDS.Sora), 'Sora')
    assert.equal(
      getChannelTypeLabel(BACKEND_CHANNEL_IDS.MiniMaxVideo),
      'MiniMax Video'
    )
  })
})

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

/**
 * Special-channel routing: which channels serve a request based on the
 * modality tags the relay derived from it.
 *
 * Each option key holds a JSON array of rules (`dto.SpecailChannels`):
 *
 *   [{"model_name": "^gemini-", "channel_ids": [[3, 4], [7]],
 *     "random_type": "order"}]
 *
 * `channel_ids` is a list of tiers; the relay shuffles within a tier and
 * concatenates tiers in order, so tier order is the fallback priority.
 * `model_name` is matched exactly first, then as a regular expression.
 *
 * These rules OVERRIDE a key's own `channel_rules` when they match — see
 * `getSpecialChannels` in `middleware/distributor.go`.
 */

/** The four option keys, in the order the distributor checks them. */
export const SPECIAL_CHANNEL_KEYS = [
  'OnlyTextChannels',
  'VideoChannels',
  'OnlyImageChannels',
  'NoVideoChannels',
] as const

export type SpecialChannelKey = (typeof SPECIAL_CHANNEL_KEYS)[number]

/** One tier of equally-weighted channels. `uid` is a stable React key. */
export type SpecialChannelTier = {
  uid: string
  ids: number[]
}

export type SpecialChannelRule = {
  uid: string
  modelName: string
  randomType: 'order' | 'random'
  /** Tiers are shuffled internally by the relay but kept in order. */
  tiers: SpecialChannelTier[]
}

let uidCounter = 0
const nextUid = () => `sc-${(uidCounter += 1)}`

function toIntList(value: unknown): number[] {
  if (!Array.isArray(value)) return []
  return value
    .map((item) => Number(item))
    .filter((item) => Number.isFinite(item) && item > 0)
}

/** Parse one option value. Invalid or non-array input yields `[]`. */
export function parseSpecialChannelRules(
  raw: string | null | undefined
): SpecialChannelRule[] {
  if (!raw || !raw.trim()) return []
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return []
  }
  if (!Array.isArray(parsed)) return []

  return parsed
    .map((entry) => {
      if (!entry || typeof entry !== 'object') return null
      const rule = entry as Record<string, unknown>
      const modelName = String(rule.model_name ?? '').trim()
      if (!modelName) return null
      const tiers = Array.isArray(rule.channel_ids)
        ? rule.channel_ids
            .map((tier) => ({ uid: nextUid(), ids: toIntList(tier) }))
            .filter((tier) => tier.ids.length > 0)
        : []
      return {
        uid: nextUid(),
        modelName,
        randomType: rule.random_type === 'random' ? 'random' : 'order',
        tiers,
      } as SpecialChannelRule
    })
    .filter((rule): rule is SpecialChannelRule => rule !== null)
}

/**
 * Serialize back to the option value.
 *
 * Rules without a model name or without any channel are dropped: the
 * distributor treats an empty id list as "no special routing", so keeping them
 * would only be a silent no-op in the UI.
 */
export function serializeSpecialChannelRules(
  rules: SpecialChannelRule[]
): string {
  const payload = rules
    .map((rule) => ({
      model_name: rule.modelName.trim(),
      channel_ids: rule.tiers
        .map((tier) => tier.ids)
        .filter((ids) => ids.length > 0),
      random_type: rule.randomType,
    }))
    .filter((rule) => rule.model_name && rule.channel_ids.length > 0)

  return JSON.stringify(payload)
}

export function createEmptySpecialChannelRule(): SpecialChannelRule {
  return {
    uid: nextUid(),
    modelName: '',
    randomType: 'order',
    tiers: [createEmptySpecialChannelTier()],
  }
}

export function createEmptySpecialChannelTier(): SpecialChannelTier {
  return { uid: nextUid(), ids: [] }
}

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
 * Channel routing rules stored on an API key as a JSON string.
 *
 * Wire format (see `dto.ChannelRulesItem`), keyed by model name or prefix:
 *
 *   {"gpt-4o-mini": {"retry": 2, "random_type": "order",
 *                    "disable_channels": [10],
 *                    "channels": [{"ids": [23, 24]},
 *                                 {"id": 25, "group_ratio": {"default": 1}}]}}
 *
 * Each entry in `channels` is one tier; the relay walks tiers in order (or
 * picks at random when `random_type` is `random`), so tier order is meaningful
 * and the editor lets it be reordered.
 */

/**
 * Identity for React keys. Rules and tiers are reordered and deleted, so an
 * array index is not a stable key; this counter is not persisted.
 */
let uidCounter = 0
const nextUid = () => `cr-${(uidCounter += 1)}`

/** A tier within a rule. `extra` carries fields this editor does not surface. */
export type ChannelRuleTier = {
  uid: string
  ids: number[]
  /**
   * Fields the backend understands but the editor does not expose
   * (`group_ratio`, `name`, `weight`). Preserved verbatim so editing a rule
   * never silently drops configuration written by hand or by an older UI.
   */
  extra: Record<string, unknown>
}

export type ChannelRule = {
  uid: string
  /** Model name or prefix this rule matches. */
  modelKey: string
  retry: number
  randomType: 'order' | 'random'
  disableChannels: number[]
  tiers: ChannelRuleTier[]
}

export const RANDOM_TYPES = ['order', 'random'] as const

/** Fields the visual editor owns; anything else on a tier is passthrough. */
const TIER_OWNED_KEYS = new Set(['id', 'ids'])

function toIntList(value: unknown): number[] {
  if (!Array.isArray(value)) return []
  return value
    .map((item) => Number(item))
    .filter((item) => Number.isFinite(item) && item > 0)
}

function tierFromJson(raw: unknown): ChannelRuleTier | null {
  if (!raw || typeof raw !== 'object') return null
  const record = raw as Record<string, unknown>

  // A tier is written either as `{ids: [...]}` or as a single `{id: n}`.
  const ids = record.ids !== undefined ? toIntList(record.ids) : []
  const singleId = Number(record.id ?? 0)
  if (ids.length === 0 && Number.isFinite(singleId) && singleId > 0) {
    ids.push(singleId)
  }
  if (ids.length === 0) return null

  const extra: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(record)) {
    if (!TIER_OWNED_KEYS.has(key)) extra[key] = value
  }
  return { uid: nextUid(), ids, extra }
}

/** Parse the stored JSON into editable rules. Invalid input yields `[]`. */
export function parseChannelRules(raw: string | null | undefined): ChannelRule[] {
  if (!raw || !raw.trim()) return []
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return []
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return []

  return Object.entries(parsed as Record<string, unknown>).map(
    ([modelKey, value]) => {
      const rule = (value ?? {}) as Record<string, unknown>
      const randomType = rule.random_type === 'random' ? 'random' : 'order'
      const retry = Number(rule.retry ?? 0)
      return {
        uid: nextUid(),
        modelKey,
        retry: Number.isFinite(retry) && retry > 0 ? Math.floor(retry) : 0,
        randomType,
        disableChannels: toIntList(rule.disable_channels),
        tiers: Array.isArray(rule.channels)
          ? rule.channels
              .map((tier) => tierFromJson(tier))
              .filter((tier): tier is ChannelRuleTier => tier !== null)
          : [],
      }
    }
  )
}

/**
 * Serialize rules back to the stored JSON string.
 *
 * Returns `''` when nothing is configured so the field round-trips to an empty
 * value rather than a literal `{}`. Rules without a model key, and tiers
 * without channels, are dropped — they cannot match anything.
 */
export function serializeChannelRules(rules: ChannelRule[]): string {
  const output: Record<string, unknown> = {}

  for (const rule of rules) {
    const modelKey = rule.modelKey.trim()
    if (!modelKey) continue

    const channels = rule.tiers
      .filter((tier) => tier.ids.length > 0)
      .map((tier) => {
        // Single-channel tiers keep the `{id}` shape when they carry
        // passthrough fields, matching what the backend writes.
        const hasExtra = Object.keys(tier.extra).length > 0
        if (hasExtra && tier.ids.length === 1) {
          return { id: tier.ids[0], ...tier.extra }
        }
        return hasExtra ? { ids: tier.ids, ...tier.extra } : { ids: tier.ids }
      })

    output[modelKey] = {
      retry: rule.retry,
      random_type: rule.randomType,
      disable_channels: rule.disableChannels,
      channels,
    }
  }

  return Object.keys(output).length > 0 ? JSON.stringify(output) : ''
}

export function createEmptyRule(): ChannelRule {
  return {
    uid: nextUid(),
    modelKey: '',
    retry: 0,
    randomType: 'order',
    disableChannels: [],
    tiers: [],
  }
}

export function createEmptyTier(): ChannelRuleTier {
  return { uid: nextUid(), ids: [], extra: {} }
}

/** Move a tier within a rule, used by the drag handle. */
export function reorderTiers(
  tiers: ChannelRuleTier[],
  from: number,
  to: number
): ChannelRuleTier[] {
  if (from === to || from < 0 || to < 0) return tiers
  if (from >= tiers.length || to >= tiers.length) return tiers
  const next = [...tiers]
  const [moved] = next.splice(from, 1)
  next.splice(to, 0, moved)
  return next
}

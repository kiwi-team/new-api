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

import { z } from 'zod'

export const SPECIAL_CHANNEL_KEYS = ['LargeFileChannels'] as const

export const geminiFileRoutingSchema = z.object({
  enabled: z.boolean().default(false),
  mode: z.enum(['forward', 'process']).default('forward'),
  chat_model_patterns: z.array(z.string().trim().min(1)).default([]),
  inline_threshold_bytes: z
    .number()
    .int()
    .min(1)
    .max(16777216)
    .default(10485760),
  max_inline_request_bytes: z.number().int().min(1).default(33554432),
})

export type GeminiFileRouting = z.infer<typeof geminiFileRoutingSchema>

export function parseGeminiFileRouting(raw?: string): GeminiFileRouting {
  return geminiFileRoutingSchema.parse(raw === undefined ? {} : JSON.parse(raw))
}

const largeFileRulesSchema = z.array(
  z.object({
    model_name: z.string().trim().min(1),
    random_type: z.enum(['order', 'random']).default('order'),
    channel_ids: z.array(z.array(z.number().int().positive()).min(1)).min(1),
  })
)

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

/** Reject malformed saved configuration instead of presenting it as empty. */
export function parseSpecialChannelRules(
  raw: string | null | undefined
): SpecialChannelRule[] {
  if (raw == null) return []
  return largeFileRulesSchema.parse(JSON.parse(raw)).map((rule) => ({
    uid: nextUid(),
    modelName: rule.model_name,
    randomType: rule.random_type,
    tiers: rule.channel_ids.map((ids) => ({ uid: nextUid(), ids })),
  }))
}

export function serializeSpecialChannelRules(
  rules: SpecialChannelRule[]
): string {
  return JSON.stringify(
    largeFileRulesSchema.parse(
      rules.map((rule) => ({
        model_name: rule.modelName.trim(),
        channel_ids: rule.tiers.map((tier) => tier.ids),
        random_type: rule.randomType,
      }))
    )
  )
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

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
import { USD_PER_1M_AT_RATIO_ONE } from '../constants'
import type {
  BillingMode,
  OfficialPriceRow,
  PriceTier,
  PricingOptions,
} from '../types'

/** Prices are stored with 6 decimals; rounding keeps currency round-trips stable. */
export function round6(value: number): number {
  return Math.round((Number(value) + Number.EPSILON) * 1e6) / 1e6
}

export function formatTokens(count: number): string {
  if (count >= 1_000_000) {
    return `${(count / 1_000_000).toFixed(count % 1_000_000 === 0 ? 0 : 1)}M`
  }
  if (count >= 1_000) {
    return `${(count / 1_000).toFixed(count % 1_000 === 0 ? 0 : 1)}K`
  }
  return String(count)
}

/**
 * Display format for a price: two decimals by default, widened only while the
 * shown number still equals the real one.
 *
 * Prices are stored as 6-decimal USD, so a CNY amount typed by the user comes
 * back through `usd * rate` with noise in the last digits (¥1 stored as
 * $0.147059 reads back as ¥1.000008). Formatting absorbs that noise without
 * distorting real prices: 0.075 stays 0.075 rather than becoming 0.08, and
 * 0.002 stays 0.002 rather than collapsing to 0.00.
 *
 * The tolerance is relative 1e-5, at least 5e-6 (so ¥0.01-scale prices, whose
 * 6-decimal USD noise is relatively large, still collapse) and at most 1% of
 * the value (so a tiny price is never snapped onto a different magnitude).
 */
export function formatPrice(value: number | null | undefined): string {
  const num = Number(value)
  if (value === null || value === undefined || !Number.isFinite(num)) {
    return '-'
  }
  if (num === 0) return '0.00'
  const abs = Math.abs(num)
  const tolerance = Math.min(Math.max(abs * 1e-5, 5e-6), abs * 1e-2)
  for (let decimals = 2; decimals <= 6; decimals++) {
    const text = num.toFixed(decimals)
    const shown = Number(text)
    if (shown !== 0 && Math.abs(shown - num) <= tolerance) return text
  }
  return String(num)
}

/**
 * Absolute floor for {@link nearlyEqual}, slightly above the 6-decimal storage
 * step: a difference smaller than one step cannot survive a round trip, so
 * treating it as "no change" is the correct semantics.
 */
const NO_OP_ABS_EPS = 1.5e-6

/**
 * Whether two USD prices are the same price.
 *
 * Stored prices are 6-decimal quantized and the backend snaps the derived ratio
 * once more, so a value recomputed from the CNY field differs from the stored
 * one in the last digits. Comparing with `!==` would make merely focusing and
 * leaving an input write a new price — and, because the write lands back on the
 * same stored value, make it look like edits never reach the database.
 *
 * The relative tolerance of 5e-5 is 0.005%, far below any meaningful repricing.
 */
export function nearlyEqual(a: number, b: number): boolean {
  const x = Number(a) || 0
  const y = Number(b) || 0
  if (x === y) return true
  const scale = Math.max(Math.abs(x), Math.abs(y))
  if (scale === 0) return true
  return Math.abs(x - y) <= Math.max(scale * 5e-5, NO_OP_ABS_EPS)
}

/** A model ratio expressed as $/1M tokens. */
export function ratioToUsdPer1M(ratio: unknown): number | null {
  const parsed = Number(ratio)
  if (ratio === undefined || ratio === null || Number.isNaN(parsed)) return null
  return parsed * USD_PER_1M_AT_RATIO_ONE
}

function parseJsonObject<T>(raw: string | undefined): Record<string, T> {
  try {
    const parsed = JSON.parse(raw || '{}')
    return parsed && typeof parsed === 'object'
      ? (parsed as Record<string, T>)
      : {}
  } catch {
    return {}
  }
}

export function parseTieredPriceMap(
  options: PricingOptions
): Record<string, PriceTier[]> {
  const map = parseJsonObject<PriceTier[]>(options.TieredPrice)
  const result: Record<string, PriceTier[]> = {}
  for (const [model, tiers] of Object.entries(map)) {
    if (Array.isArray(tiers)) result[model] = tiers
  }
  return result
}

/**
 * Fold the pricing options into one row per model.
 *
 * Ratios and absolute prices live in separate options, and a model is only
 * ever in one billing mode: a tiered entry wins over a per-call price, which
 * wins over the per-token ratios.
 */
export function buildOfficialPriceRows(
  options: PricingOptions
): OfficialPriceRow[] {
  const modelRatio = parseJsonObject<number>(options.ModelRatio)
  const completionRatio = parseJsonObject<number>(options.CompletionRatio)
  const modelPrice = parseJsonObject<number>(options.ModelPrice)
  const cacheRatio = parseJsonObject<number>(options.CacheRatio)
  const createCacheRatio = parseJsonObject<number>(options.CreateCacheRatio)
  const tieredPrice = parseTieredPriceMap(options)
  const updateTime = parseJsonObject<number>(options.ModelPriceUpdateTime)

  const names = new Set([
    ...Object.keys(modelRatio),
    ...Object.keys(completionRatio),
    ...Object.keys(modelPrice),
    ...Object.keys(tieredPrice),
  ])

  const rows = [...names].map((model) => {
    let billingMode: BillingMode = 'token'
    if (Array.isArray(tieredPrice[model])) billingMode = 'tiered'
    else if (modelPrice[model] !== undefined) billingMode = 'call'

    const ratio = modelRatio[model]
    const completion = completionRatio[model]
    const inputUSD = ratio === undefined ? null : round6(ratio * 2)
    const outputUSD =
      ratio === undefined
        ? null
        : round6(ratio * (completion === undefined ? 1 : completion) * 2)

    // Cache options are multipliers of the input price, shown as absolute prices.
    const readRatio = cacheRatio[model]
    const createRatio = createCacheRatio[model]
    return {
      model,
      billingMode,
      inputUSD,
      outputUSD,
      perCallUSD: billingMode === 'call' ? (modelPrice[model] ?? null) : null,
      cacheReadUSD:
        readRatio !== undefined && inputUSD ? round6(readRatio * inputUSD) : null,
      cacheCreateUSD:
        createRatio !== undefined && inputUSD
          ? round6(createRatio * inputUSD)
          : null,
      tiers: tieredPrice[model] ?? [],
      updatedAt: updateTime[model] ?? null,
    } satisfies OfficialPriceRow
  })

  // Most recently edited first, so a just-saved model stays in view.
  rows.sort((a, b) => {
    if ((b.updatedAt || 0) !== (a.updatedAt || 0)) {
      return (b.updatedAt || 0) - (a.updatedAt || 0)
    }
    return a.model.localeCompare(b.model)
  })
  return rows
}

/** Official price lookup used to contrast customer discounts against list price. */
export function buildOfficialPriceMap(
  options: PricingOptions
): Record<string, { input: number | null; output: number | null }> {
  const map: Record<string, { input: number | null; output: number | null }> = {}
  for (const row of buildOfficialPriceRows(options)) {
    map[row.model] = { input: row.inputUSD, output: row.outputUSD }
  }
  return map
}

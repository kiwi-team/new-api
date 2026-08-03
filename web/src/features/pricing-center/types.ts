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
/** How a model is charged. A model holds exactly one of these three shapes. */
export type BillingMode = 'token' | 'call' | 'tiered'

/** One step of a tiered price, keyed by the input-token ceiling it applies to. */
export type PriceTier = {
  max_tokens: number
  input_price: number
  output_price: number
  cached_input_price?: number
  cache_write_price?: number
}

/** A row of the official price table, derived from the pricing options. */
export type OfficialPriceRow = {
  model: string
  billingMode: BillingMode
  /** $/1M input tokens; null when the model has no ratio configured */
  inputUSD: number | null
  outputUSD: number | null
  perCallUSD: number | null
  cacheReadUSD: number | null
  cacheCreateUSD: number | null
  tiers: PriceTier[]
  /** Unix seconds of the last price edit, from `ModelPriceUpdateTime` */
  updatedAt: number | null
}

/** Raw option values the official price table is built from. */
export type PricingOptions = {
  ModelRatio?: string
  CompletionRatio?: string
  ModelPrice?: string
  CacheRatio?: string
  CreateCacheRatio?: string
  TieredPrice?: string
  ModelPriceUpdateTime?: string
}

export type UpdateModelPricingPayload = {
  model_name: string
  is_per_call: boolean
  input_price: number
  output_price: number
  per_call_price: number
  cache_read_price: number
  cache_create_price: number
}

export type ApiResponse<T = unknown> = {
  success: boolean
  message?: string
  data?: T
}

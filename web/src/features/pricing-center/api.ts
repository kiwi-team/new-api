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
import { api } from '@/lib/api'

import type {
  ApiResponse,
  PriceTier,
  PricingOptions,
  UpdateModelPricingPayload,
} from './types'

// ============================================================================
// Official prices — stored as system options, edited through /api/pricing
// ============================================================================

/**
 * Load the option map the price table is derived from. Options come back as a
 * flat `{key, value}` list; only the pricing keys are of interest here.
 */
export async function getPricingOptions(): Promise<PricingOptions> {
  const res = await api.get('/api/option/')
  const items = (res.data?.data ?? []) as { key: string; value: string }[]
  const options: Record<string, string> = {}
  for (const item of items) {
    options[item.key] = item.value
  }
  return options as PricingOptions
}

export async function updateModelPricing(
  payload: UpdateModelPricingPayload
): Promise<ApiResponse> {
  const res = await api.put('/api/pricing/model', payload)
  return res.data
}

export async function deleteModelPricing(
  modelName: string
): Promise<ApiResponse> {
  const res = await api.delete('/api/pricing/model', {
    params: { model_name: modelName },
  })
  return res.data
}

/**
 * Tiered prices live in the `TieredPrice` option as `{model: tiers[]}`, so a
 * single model's tiers are written by rewriting the whole map.
 */
export async function saveTieredPriceMap(
  map: Record<string, PriceTier[]>
): Promise<ApiResponse> {
  const res = await api.put('/api/option/', {
    key: 'TieredPrice',
    value: JSON.stringify(map, null, 2),
  })
  return res.data
}

export async function getExchangeRate(): Promise<number | null> {
  const res = await api.get('/api/status')
  const rate = res.data?.data?.usd_exchange_rate
  const parsed = Number(rate)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null
}

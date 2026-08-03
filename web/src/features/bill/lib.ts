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
import { USD_PER_1M_AT_RATIO_ONE } from '@/features/pricing-center/constants'

import type { SelfSettlementConfig } from './types'

/** Default query window when the page opens. */
export const DEFAULT_BILL_RANGE_DAYS = 7

export function daysAgo(days: number): Date {
  const date = new Date()
  date.setDate(date.getDate() - days)
  date.setHours(0, 0, 0, 0)
  return date
}

/** Bill amounts are settled in dollars and shown with full precision. */
export function formatAmount(value: number | null | undefined): string {
  if (value === null || value === undefined) return '-'
  return Number(value).toFixed(6)
}

export function formatCount(value: number | null | undefined): string {
  if (value === null || value === undefined) return '-'
  return Number(value).toLocaleString()
}

export function formatPrice(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(Number(value))) {
    return '0'
  }
  return Number(value).toFixed(4)
}

/**
 * List price of a model in $/1M tokens, derived from the site ratios. Only
 * per-token models have one — per-call models and models the site never
 * configured show as unavailable.
 */
export function siteListPrices(config: SelfSettlementConfig): {
  input: number | null
  output: number | null
} {
  if (!config.site_configured || config.site_is_per_call) {
    return { input: null, output: null }
  }
  const modelRatio = config.site_model_ratio || 0
  const completionRatio = config.site_completion_ratio || 0
  return {
    input: modelRatio * USD_PER_1M_AT_RATIO_ONE,
    output: modelRatio * completionRatio * USD_PER_1M_AT_RATIO_ONE,
  }
}

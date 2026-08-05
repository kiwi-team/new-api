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
import type { TFunction } from 'i18next'

import type { BillingMode } from './types'

/** Fallback USD→CNY rate used until `/api/status` reports the configured one. */
export const DEFAULT_EXCHANGE_RATE = 7.3

/**
 * A ratio of 1 means $2 per 1M tokens, so `$/1M = ratio × 2`. This is the
 * system-wide baseline the ratio options are expressed against.
 */
export const USD_PER_1M_AT_RATIO_ONE = 2

/** Discount bounds mirror `SettlementDiscountMin/Max` in the backend model. */
export const DISCOUNT_MIN = 0.01
export const DISCOUNT_MAX = 10

export const BILLING_MODES: BillingMode[] = ['token', 'call']

export function getBillingModeLabel(mode: BillingMode, t: TFunction): string {
  switch (mode) {
    case 'call':
      return t('Per-call billing')
    case 'legacy-tiered':
      return t('Legacy tiered pricing')
    case 'expression':
      return t('Expression billing')
    default:
      return t('Per-token billing')
  }
}

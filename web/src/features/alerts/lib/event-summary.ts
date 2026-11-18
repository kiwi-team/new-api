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

import { formatUSD } from '@/lib/currency'
import { formatNumber } from '@/lib/format'
import { DEFAULT_CURRENCY_CONFIG } from '@/stores/system-config-store'

import { alertLabel } from './labels'
import { formatAlertMetricValue, isPercentageMetric } from './rule-form'

export function formatAlertMoney(
  value: string,
  locale: Intl.LocalesArgument,
  quotaUnit = DEFAULT_CURRENCY_CONFIG.quotaPerUnit
): string {
  if (
    !/^-?\d+$/.test(value) ||
    !Number.isSafeInteger(quotaUnit) ||
    quotaUnit <= 0
  ) {
    return value
  }
  const quota = BigInt(value),
    unit = BigInt(quotaUnit)
  const magnitude = quota < 0n ? -quota : quota
  const sign = quota < 0n ? '-' : ''
  const fraction = (((magnitude % unit) * 100000000n) / unit)
    .toString()
    .padStart(8, '0')
  const fixed = `${sign}${magnitude / unit}.${fraction}`
  const amount = Number(value) / quotaUnit
  if (Number.isSafeInteger(Number(value)) && amount.toFixed(8) === fixed) {
    return formatUSD(amount, {
      digitsLarge: 8,
      digitsSmall: 8,
      abbreviate: false,
      locale,
    })
  }
  // Keep int64 amounts exact instead of rounding through a JavaScript number.
  const decimals = fraction.replace(/0+$/, '')
  return `${sign}${magnitude / unit}${decimals ? `.${decimals}` : ''} USD`
}

// Format only the engine's trailing measurement, preserving names and legacy
// notice text. Stored summaries keep their original values and audit identity.
export function formatAlertEventSummary(
  summary: string,
  t: TFunction,
  locale: Intl.LocalesArgument,
  quotaUnit = DEFAULT_CURRENCY_CONFIG.quotaPerUnit
): string {
  return summary.replace(
    / \| ([a-z_]+)=(-?\d+) \| samples=(\d+)$/,
    (original, metric: string, value: string, samples: string) => {
      const money = metric === 'spend_net' || metric === 'balance_remaining'
      if (!money && !isPercentageMetric(metric)) return original
      const displayValue = money
        ? formatAlertMoney(value, locale, quotaUnit)
        : formatAlertMetricValue(metric, value, locale, { samples })
      const sampleCount = Number.isSafeInteger(Number(samples))
        ? formatNumber(Number(samples), locale)
        : samples
      return ` | ${alertLabel(metric, t)}: ${displayValue} | ${t('Valid samples')}: ${sampleCount}`
    }
  )
}

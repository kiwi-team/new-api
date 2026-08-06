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
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'

import type { ClientUserQuota, ProjectBudgetSummary } from '../types'

/** Budgets are stored in USD with 6 decimals; anything coarser hides real spend. */
const budgetFormat = { digitsLarge: 6, digitsSmall: 6 } as const

function BudgetRow({
  label,
  value,
  className,
}: {
  label: string
  value: string
  className?: string
}) {
  return (
    <div className='flex gap-1'>
      <span className='text-muted-foreground'>{label}</span>
      <span className={className}>{value}</span>
    </div>
  )
}

/**
 * Non-project budget for one client UID.
 *
 * The fixed and temporary budgets only ever cover untagged spend — project spend
 * is charged against each project's own allocation, and the two pools never draw
 * on each other. `monthly_non_project_used_usd` is the same figure the backend
 * admission gate compares against, so what this cell shows as available is
 * exactly what the next untagged request will be allowed to spend.
 */
export function NonProjectBudgetCell({
  quota,
  summary,
}: {
  quota: ClientUserQuota
  summary: ProjectBudgetSummary | undefined
}) {
  const { t } = useTranslation()

  // Without the rollup we cannot know how much of the budget is already spent.
  // Showing $0 spent would read as "full budget available" — the one wrong
  // answer to give about a budget — so both derived rows fall back to a dash.
  const nonProjectUsed = summary?.monthly_non_project_used_usd
  const fixedBudget = quota.fixed_quota || 0
  const tempBudget = quota.temp_quota || 0
  const tempExpired = Boolean(
    quota.expired_at && quota.expired_at <= Date.now() / 1000
  )

  let usedText = '—'
  let availableText = '—'
  let availableClassName: string | undefined
  if (nonProjectUsed !== undefined) {
    const available = Math.max(
      fixedBudget + (tempExpired ? 0 : tempBudget) - nonProjectUsed,
      0
    )
    usedText = formatCurrencyFromUSD(nonProjectUsed, budgetFormat)
    availableText = formatCurrencyFromUSD(available, budgetFormat)
    availableClassName =
      available > 0 ? 'font-medium text-success' : 'font-medium text-destructive'
  }

  return (
    <div className='space-y-0.5 text-sm'>
      <BudgetRow
        label={t('Monthly fixed budget:')}
        value={formatCurrencyFromUSD(fixedBudget, budgetFormat)}
      />
      <div className='flex flex-wrap items-center gap-1'>
        <span className='text-muted-foreground'>{t('Temporary budget:')}</span>
        <span>{formatCurrencyFromUSD(tempBudget, budgetFormat)}</span>
        {tempBudget > 0 && (
          <span className='text-muted-foreground'>
            {quota.expired_at
              ? t('(expires {{time}})', {
                  time: formatTimestampToDate(quota.expired_at),
                })
              : t('(no expiry)')}
          </span>
        )}
        {tempExpired && <Badge variant='destructive'>{t('Expired')}</Badge>}
      </div>
      <BudgetRow label={t('Non-project spend this month:')} value={usedText} />
      <BudgetRow
        label={t('Available:')}
        value={availableText}
        className={availableClassName}
      />
    </div>
  )
}

/**
 * Currently effective project budget for one client UID: allocated / used /
 * available. Rendered inside a link button so the whole block opens the
 * allocation breakdown — with no effective plan the numbers are all zero, but
 * the dialog still has history worth reading, so the cell stays clickable.
 */
export function ProjectBudgetCell({
  summary,
}: {
  summary: ProjectBudgetSummary | undefined
}) {
  const { t } = useTranslation()

  if (!summary?.projects?.length) {
    return (
      <span className='text-primary underline-offset-4 hover:underline'>
        {t('No effective budget')}
      </span>
    )
  }

  return (
    <div className='space-y-0.5 text-sm text-primary'>
      <BudgetRow
        label={t('Allocated:')}
        value={formatCurrencyFromUSD(summary.total_allocated, budgetFormat)}
      />
      <BudgetRow
        label={t('Used:')}
        value={formatCurrencyFromUSD(summary.total_used_usd, budgetFormat)}
      />
      <BudgetRow
        label={t('Available:')}
        value={formatCurrencyFromUSD(summary.total_remaining_usd, budgetFormat)}
        className='font-medium'
      />
    </div>
  )
}

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
import { formatTimestampToDate, quotaUnitsToDollars } from '@/lib/format'

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
 * The UID's `used_quota` counts *all* spend this month, project and otherwise.
 * Project spend is tracked separately per allocation, so the non-project figure
 * is the difference — without subtracting it, a UID that spent everything
 * through projects would still look like it had burned its fixed budget.
 */
export function NonProjectBudgetCell({
  quota,
  summary,
}: {
  quota: ClientUserQuota
  summary: ProjectBudgetSummary | undefined
}) {
  const { t } = useTranslation()

  const totalUsed = quotaUnitsToDollars(quota.used_quota ?? 0)
  const nonProjectUsed = Math.max(
    totalUsed - (summary?.monthly_project_used_usd ?? 0),
    0
  )
  const fixedBudget = quota.fixed_quota || 0
  const tempBudget = quota.temp_quota || 0
  const tempExpired = Boolean(
    quota.expired_at && quota.expired_at <= Date.now() / 1000
  )
  const available = Math.max(
    fixedBudget + (tempExpired ? 0 : tempBudget) - nonProjectUsed,
    0
  )

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
      <BudgetRow
        label={t('Non-project spend this month:')}
        value={formatCurrencyFromUSD(nonProjectUsed, budgetFormat)}
      />
      <BudgetRow
        label={t('Available:')}
        value={formatCurrencyFromUSD(available, budgetFormat)}
        className={
          available > 0 ? 'font-medium text-success' : 'font-medium text-destructive'
        }
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

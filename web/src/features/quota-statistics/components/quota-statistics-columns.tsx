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

import type { StaticDataTableColumn } from '@/components/data-table'
import { cn } from '@/lib/utils'

import { budgetUsageLevel, isClaudeModel } from '../lib'
import type { QuotaStatisticsRow } from '../types'

type ExpandFlags = {
  models: boolean
  dates: boolean
  tokens: boolean
}

function formatTokens(value: number | undefined): string {
  return String(Number.parseInt(String(value ?? 0), 10) || 0)
}

function formatCost(value: number | undefined): string {
  return (Number(value) || 0).toFixed(6)
}

/**
 * Columns follow the expand flags: without them the table is one row per UID
 * with its monthly budget; expanding swaps in the date / model / key columns.
 */
export function useQuotaStatisticsColumns(
  expand: ExpandFlags
): StaticDataTableColumn<QuotaStatisticsRow>[] {
  const { t } = useTranslation()

  const cacheCreationCell =
    (tokensKey: keyof QuotaStatisticsRow, costKey: keyof QuotaStatisticsRow) =>
    (row: QuotaStatisticsRow) => {
      if (expand.models && row.model_name && !isClaudeModel(row.model_name)) {
        return '-'
      }
      return (
        <div className='flex flex-col leading-tight'>
          <span>{formatTokens(row[tokensKey] as number)}</span>
          <span className='text-muted-foreground text-xs'>
            ${formatCost(row[costKey] as number)}
          </span>
        </div>
      )
    }

  return [
    ...(expand.dates
      ? [
          {
            id: 'date',
            header: t('Date'),
            cell: (row: QuotaStatisticsRow) => row.date ?? '-',
          },
        ]
      : []),
    {
      id: 'client_user_id',
      header: t('Client UID'),
      cellClassName: 'font-medium',
      cell: (row) => row.client_user_id || '-',
    },
    ...(expand.models
      ? [
          {
            id: 'model_name',
            header: t('Model name'),
            cell: (row: QuotaStatisticsRow) => row.model_name ?? '-',
          },
        ]
      : [
          {
            id: 'month_budget',
            header: t('Monthly budget'),
            cell: (row: QuotaStatisticsRow) => {
              const fixed = Number.parseInt(String(row.fixed_quota ?? 0), 10) || 0
              const temp = Number.parseInt(String(row.temp_quota ?? 0), 10) || 0
              return (
                <span>
                  {fixed + temp}
                  <span className='text-muted-foreground ms-1 text-xs'>
                    ({t('fixed')} {fixed} + {t('temporary')} {temp})
                  </span>
                </span>
              )
            },
          },
        ]),
    ...(expand.tokens
      ? [
          {
            id: 'token',
            header: t('Key'),
            cell: (row: QuotaStatisticsRow) =>
              `${row.token_name ?? ''}(${row.token_id ?? ''})`,
          },
        ]
      : []),
    {
      id: 'total_quota',
      header: t('Consumption ($)'),
      cellClassName: (row: QuotaStatisticsRow) => {
        const level = budgetUsageLevel(row)
        return cn(
          level !== 'none' && 'font-semibold',
          level === 'warning' && 'text-amber-600 dark:text-amber-400',
          level === 'exceeded' && 'text-destructive'
        )
      },
      cell: (row) => (Number(row.total_quota) || 0).toFixed(6),
    },
    {
      id: 'total_count',
      header: t('Requests'),
      cell: (row) => row.total_count ?? 0,
    },
    {
      id: 'total_prompt',
      header: t('Total prompt tokens'),
      cell: (row) => row.total_prompt ?? 0,
    },
    {
      id: 'total_completion',
      header: t('Total completion tokens'),
      cell: (row) => row.total_completion ?? 0,
    },
    {
      id: 'total_cached_tokens',
      header: t('Cache read tokens'),
      cell: (row) => formatTokens(row.total_cached_tokens),
    },
    {
      id: 'total_cache_cost',
      header: t('Cache read ($)'),
      cell: (row) => formatCost(row.total_cache_cost),
    },
    {
      id: 'cache_creation_5m',
      header: t('Cache write (Claude, 5m)'),
      cell: cacheCreationCell(
        'total_cache_creation_5m_tokens',
        'total_cache_creation_5m_cost'
      ),
    },
    {
      id: 'cache_creation_1h',
      header: t('Cache write (Claude, 1h)'),
      cell: cacheCreationCell(
        'total_cache_creation_1h_tokens',
        'total_cache_creation_1h_cost'
      ),
    },
  ]
}

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
import type { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatTimestampToDate, quotaUnitsToDollars } from '@/lib/format'

import type { ClientUserQuota, ProjectBudgetSummary } from '../types'
import { ClientUserQuotaRowActions } from './data-table-row-actions'

type ColumnsOptions = {
  /** Client names are admin-only, matching the backend's org scoping */
  showClientName: boolean
  projectBudgets: Record<string, ProjectBudgetSummary>
  onOpenProjectBudget: (clientUserId: string) => void
}

export function useClientUserQuotaColumns({
  showClientName,
  projectBudgets,
  onOpenProjectBudget,
}: ColumnsOptions): ColumnDef<ClientUserQuota>[] {
  const { t } = useTranslation()

  return [
    {
      accessorKey: 'client_user_id',
      header: t('Client UID'),
      meta: { mobileTitle: true },
      cell: ({ row }) => (
        <span className='font-medium'>{row.original.client_user_id}</span>
      ),
      size: 220,
    },
    ...(showClientName
      ? [
          {
            accessorKey: 'client_name',
            header: t('Client name'),
            cell: ({ row }) => row.original.client_name || '-',
            size: 150,
          } satisfies ColumnDef<ClientUserQuota>,
        ]
      : []),
    {
      accessorKey: 'fixed_quota',
      header: t('Monthly fixed budget'),
      cell: ({ row }) => formatCurrencyFromUSD(row.original.fixed_quota),
      size: 140,
    },
    {
      accessorKey: 'temp_quota',
      header: t('Temporary budget'),
      cell: ({ row }) => formatCurrencyFromUSD(row.original.temp_quota),
      size: 120,
    },
    {
      accessorKey: 'used_quota',
      header: t('Used this month'),
      cell: ({ row }) =>
        formatCurrencyFromUSD(quotaUnitsToDollars(row.original.used_quota ?? 0), { digitsLarge: 6, digitsSmall: 6 }),
      size: 160,
    },
    {
      accessorKey: 'expired_at',
      header: t('Temporary budget expires at'),
      meta: { mobileHidden: true },
      cell: ({ row }) =>
        row.original.expired_at
          ? formatTimestampToDate(row.original.expired_at)
          : '-',
      size: 200,
    },
    {
      id: 'project_budget',
      header: t('Project budget'),
      enableSorting: false,
      cell: ({ row }) => {
        const summary = projectBudgets[row.original.client_user_id]
        if (!summary?.projects?.length) {
          return <span className='text-muted-foreground'>-</span>
        }
        return (
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant='link'
                  size='sm'
                  className='h-auto p-0'
                  onClick={() => onOpenProjectBudget(row.original.client_user_id)}
                />
              }
            >
              {formatCurrencyFromUSD(summary.total_allocated)}
              {t('({{count}} projects)', { count: summary.projects.length })}
            </TooltipTrigger>
            <TooltipContent>
              <div className='space-y-0.5'>
                {summary.projects.map((project) => (
                  <div key={project.project_id}>
                    {project.project_name}:{' '}
                    {formatCurrencyFromUSD(project.allocated_quota)}
                  </div>
                ))}
              </div>
            </TooltipContent>
          </Tooltip>
        )
      },
      size: 200,
    },
    {
      id: 'actions',
      header: t('Actions'),
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => <ClientUserQuotaRowActions row={row} />,
      size: 120,
    },
  ]
}

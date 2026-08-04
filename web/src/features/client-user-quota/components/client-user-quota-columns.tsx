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
import { quotaUnitsToDollars } from '@/lib/format'

import type { ClientUserQuota, ProjectBudgetSummary } from '../types'
import { NonProjectBudgetCell, ProjectBudgetCell } from './budget-cells'
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
      id: 'non_project_budget',
      header: t('Monthly non-project budget'),
      enableSorting: false,
      cell: ({ row }) => (
        <NonProjectBudgetCell
          quota={row.original}
          summary={projectBudgets[row.original.client_user_id]}
        />
      ),
      size: 300,
    },
    {
      accessorKey: 'used_quota',
      header: t('Total used this month'),
      cell: ({ row }) =>
        formatCurrencyFromUSD(quotaUnitsToDollars(row.original.used_quota ?? 0), {
          digitsLarge: 6,
          digitsSmall: 6,
        }),
      size: 160,
    },
    {
      id: 'project_budget',
      header: t('Current project budget'),
      enableSorting: false,
      cell: ({ row }) => {
        const summary = projectBudgets[row.original.client_user_id]
        return (
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant='link'
                  size='sm'
                  className='h-auto justify-start p-0 whitespace-normal'
                  onClick={() =>
                    onOpenProjectBudget(row.original.client_user_id)
                  }
                />
              }
            >
              <ProjectBudgetCell summary={summary} />
            </TooltipTrigger>
            <TooltipContent>
              {summary?.projects?.length ? (
                <div className='space-y-1'>
                  {summary.projects.map((project) => (
                    <div key={project.allocation_id}>
                      <div>
                        {project.project_name} / {project.plan_name}
                      </div>
                      <div className='text-muted-foreground'>
                        {t('Allocated {{allocated}}, used {{used}}, available {{available}}', {
                          allocated: formatCurrencyFromUSD(
                            project.allocated_quota,
                            { digitsLarge: 6, digitsSmall: 6 }
                          ),
                          used: formatCurrencyFromUSD(project.used_quota_usd, {
                            digitsLarge: 6,
                            digitsSmall: 6,
                          }),
                          available: formatCurrencyFromUSD(
                            project.remaining_quota_usd,
                            { digitsLarge: 6, digitsSmall: 6 }
                          ),
                        })}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                t('Click to view all project budgets, including history')
              )}
            </TooltipContent>
          </Tooltip>
        )
      },
      size: 240,
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

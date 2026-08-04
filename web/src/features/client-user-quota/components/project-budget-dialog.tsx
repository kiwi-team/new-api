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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatPlanDate } from '@/lib/format'

import { getProjectAllocations } from '../api'

import { AllocationStatusBadge } from './allocation-status-badge'

type ProjectBudgetDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  clientUserId: string
}

/** Budgets are stored in USD with 6 decimals; anything coarser hides real spend. */
const budgetFormat = { digitsLarge: 6, digitsSmall: 6 } as const

/**
 * Read-only breakdown of every project allocation behind one client UID,
 * including plans that are not currently effective — the list view only counts
 * effective ones, so this dialog is where history and upcoming plans surface.
 */
export function ProjectBudgetDialog({
  open,
  onOpenChange,
  clientUserId,
}: ProjectBudgetDialogProps) {
  const { t } = useTranslation()

  const { data } = useQuery({
    queryKey: ['client-user-quota', 'project-allocations', clientUserId],
    queryFn: () => getProjectAllocations(clientUserId),
    enabled: open,
  })

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={`${t('Project budget details (including history)')} - ${clientUserId}`}
      description={t(
        '"Currently effective" requires all three: the project is enabled, the plan is selected as active, and today falls inside the budget period.'
      )}
      contentClassName='sm:max-w-4xl'
      contentHeight='auto'
    >
      <StaticDataTable
        data={data ?? []}
        getRowKey={(allocation) => allocation.allocation_id}
        emptyContent={t('No project budget allocated')}
        emptyClassName='text-muted-foreground py-8'
        columns={[
          {
            id: 'project',
            header: t('Project name'),
            cellClassName: 'font-medium',
            cell: (allocation) => allocation.project_name,
          },
          {
            id: 'plan',
            header: t('Plan'),
            cell: (allocation) => allocation.plan_name || '-',
          },
          {
            id: 'period',
            header: t('Budget period'),
            cell: (allocation) =>
              `${formatPlanDate(allocation.start_date)} ~ ${formatPlanDate(allocation.end_date)}`,
          },
          {
            id: 'status',
            header: t('Status'),
            cell: (allocation) => (
              <AllocationStatusBadge allocation={allocation} />
            ),
          },
          {
            id: 'allocated',
            header: t('Allocated budget'),
            cell: (allocation) =>
              formatCurrencyFromUSD(allocation.allocated_quota, budgetFormat),
          },
          {
            id: 'used',
            header: t('Used'),
            cell: (allocation) =>
              formatCurrencyFromUSD(allocation.used_quota_usd, budgetFormat),
          },
          {
            id: 'remaining',
            header: t('Available'),
            cellClassName: 'font-medium',
            cell: (allocation) =>
              formatCurrencyFromUSD(
                allocation.remaining_quota_usd,
                budgetFormat
              ),
          },
        ]}
      />
    </Dialog>
  )
}

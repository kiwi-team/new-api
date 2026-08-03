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

import { getProjectAllocations } from '../api'

type ProjectBudgetDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  clientUserId: string
}

/** Read-only breakdown of the project allocations behind one client UID. */
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
      title={`${t('Project budget details')} - ${clientUserId}`}
      contentClassName='sm:max-w-xl'
      contentHeight='auto'
    >
      <StaticDataTable
        data={data ?? []}
        getRowKey={(allocation) => allocation.project_id}
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
            id: 'allocated',
            header: t('Allocated budget'),
            cell: (allocation) =>
              formatCurrencyFromUSD(allocation.allocated_quota),
          },
          {
            id: 'used',
            header: t('Used'),
            cell: (allocation) =>
              formatCurrencyFromUSD(Number(allocation.used_quota_usd) || 0, {
                digitsLarge: 6,
                digitsSmall: 6,
              }),
          },
        ]}
      />
    </Dialog>
  )
}

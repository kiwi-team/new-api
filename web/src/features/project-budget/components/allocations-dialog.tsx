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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatTimestampToDate, quotaUnitsToDollars } from '@/lib/format'
import { cn } from '@/lib/utils'

import { clearAllocationBudget, getPlanAllocations } from '../api'
import { formatPlanDate } from '../constants'
import type { PlanAllocation, Project, ProjectPlan } from '../types'
import { AllocationFormDialog } from './allocation-form-dialog'
import { PlanStatusBadge } from './plan-status-badge'

/** One page of allocations is plenty; the plan total comes from the API rows. */
const ALLOCATIONS_PAGE_SIZE = 100

type AllocationsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  project: Project
  plan: ProjectPlan
  onChanged: () => void
}

export function AllocationsDialog({
  open,
  onOpenChange,
  project,
  plan,
  onChanged,
}: AllocationsDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [formState, setFormState] = useState<{
    allocation?: PlanAllocation
  } | null>(null)
  const [clearTarget, setClearTarget] = useState<PlanAllocation | null>(null)

  const queryKey = ['project-budget', 'allocations', plan.id]
  const { data } = useQuery({
    queryKey,
    queryFn: () =>
      getPlanAllocations(plan.id, { p: 1, page_size: ALLOCATIONS_PAGE_SIZE }),
    enabled: open,
  })

  const allocations = data?.data?.items ?? []
  const planAllocatedTotal = allocations.reduce(
    (total, allocation) => total + (allocation.allocated_quota || 0),
    0
  )

  const refreshAllocations = async () => {
    await queryClient.invalidateQueries({ queryKey })
    onChanged()
  }

  const handleClear = async (allocation: PlanAllocation) => {
    setClearTarget(null)
    const res = await clearAllocationBudget(allocation.id)
    if (!res.success) {
      toast.error(res.message || t('Operation failed'))
      return
    }
    toast.success(t('Cleared'))
    await refreshAllocations()
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={`${t('Budget allocation')} - ${plan.plan_name} (${formatPlanDate(
        plan.start_date
      )} ~ ${formatPlanDate(plan.end_date)})`}
      contentClassName='sm:max-w-4xl'
      contentHeight='auto'
    >
      <div className='space-y-4'>
        <div className='text-muted-foreground flex flex-wrap items-center gap-6 text-sm'>
          <span>
            {t('Project total budget')}:{' '}
            {formatCurrencyFromUSD(project.total_budget)}
          </span>
          <span>
            {t('Allocated in this plan')}:{' '}
            {formatCurrencyFromUSD(planAllocatedTotal)}
          </span>
          <span className='flex items-center gap-1'>
            {t('Plan status')}:{' '}
            <PlanStatusBadge
              isActive={plan.is_active}
              isExpired={plan.is_expired}
            />
          </span>
        </div>

        <div className='flex items-center justify-between'>
          <span className='text-sm font-medium'>{t('Allocation list')}</span>
          <Button size='sm' onClick={() => setFormState({})}>
            <Plus className='h-4 w-4' />
            {t('Create allocation')}
          </Button>
        </div>

        <StaticDataTable
          tableClassName='min-w-max'
          data={allocations}
          getRowKey={(allocation) => allocation.id}
          emptyContent={t('No allocations yet')}
          emptyClassName='text-muted-foreground py-8'
          columns={[
            { id: 'id', header: t('ID'), cell: (a) => a.id },
            {
              id: 'client-uid',
              header: t('Client UID'),
              cellClassName: 'font-medium',
              cell: (a) => a.client_user_id,
            },
            {
              id: 'allocated',
              header: t('Allocated budget'),
              cell: (a) => formatCurrencyFromUSD(a.allocated_quota),
            },
            {
              id: 'used',
              header: t('Used'),
              cell: (a) =>
                formatCurrencyFromUSD(quotaUnitsToDollars(a.used_quota ?? 0), {
                  digitsLarge: 6,
                  digitsSmall: 6,
                }),
            },
            {
              id: 'remaining',
              header: t('Remaining budget'),
              cell: (a) => {
                const remaining =
                  (a.allocated_quota || 0) - quotaUnitsToDollars(a.used_quota || 0)
                return (
                  <span
                    className={cn(
                      remaining > 0
                        ? 'text-emerald-600 dark:text-emerald-400'
                        : 'text-destructive'
                    )}
                  >
                    {formatCurrencyFromUSD(remaining, {
                      digitsLarge: 6,
                      digitsSmall: 6,
                    })}
                  </span>
                )
              },
            },
            {
              id: 'created-at',
              header: t('Created at'),
              cell: (a) =>
                a.created_at ? formatTimestampToDate(a.created_at) : '-',
            },
            {
              id: 'actions',
              header: t('Actions'),
              cell: (a) => (
                <div className='flex gap-1'>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => setFormState({ allocation: a })}
                  >
                    {t('Edit')}
                  </Button>
                  <Button
                    variant='ghost'
                    size='sm'
                    className='text-destructive'
                    onClick={() => setClearTarget(a)}
                  >
                    {t('Clear')}
                  </Button>
                </div>
              ),
            },
          ]}
        />
      </div>

      {formState !== null && (
        <AllocationFormDialog
          open
          onOpenChange={(isOpen) => !isOpen && setFormState(null)}
          planId={plan.id}
          currentAllocation={formState.allocation}
          onSaved={() => void refreshAllocations()}
        />
      )}

      <ConfirmDialog
        open={clearTarget !== null}
        onOpenChange={(isOpen) => !isOpen && setClearTarget(null)}
        title={t('Clear budget')}
        desc={t(
          'Clear the remaining budget of this client? The allocated amount will be set to 0.'
        )}
        destructive
        handleConfirm={() => {
          if (clearTarget) void handleClear(clearTarget)
        }}
      />
    </Dialog>
  )
}

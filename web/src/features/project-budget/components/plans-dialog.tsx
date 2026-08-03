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
import { cn } from '@/lib/utils'

import { deleteProjectPlan, getProjectPlans } from '../api'
import { formatPlanDate } from '../constants'
import type { Project, ProjectPlan } from '../types'
import { PlanStatusBadge } from './plan-status-badge'
import { useProjectBudget } from './project-budget-provider'

type PlansDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  project: Project
}

/** Plan list of one project, with entry points into plan and allocation edits. */
export function PlansDialog({ open, onOpenChange, project }: PlansDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { setOpen, setCurrentPlan, triggerRefresh } = useProjectBudget()
  const [deleteTarget, setDeleteTarget] = useState<ProjectPlan | null>(null)

  const { data } = useQuery({
    queryKey: ['project-budget', 'plans', project.id],
    queryFn: () => getProjectPlans(project.id),
    enabled: open,
  })

  const plans = data ?? []
  const remaining = (project.total_budget || 0) - (project.allocated_total || 0)

  const handleDelete = async (plan: ProjectPlan) => {
    setDeleteTarget(null)
    const res = await deleteProjectPlan(plan.id)
    if (!res.success) {
      toast.error(res.message || t('Operation failed'))
      return
    }
    toast.success(t('Deleted successfully'))
    await queryClient.invalidateQueries({
      queryKey: ['project-budget', 'plans', project.id],
    })
    triggerRefresh()
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={`${t('Allocation plans')} - ${project.project_name}`}
      contentClassName='sm:max-w-5xl'
      contentHeight='auto'
    >
      <div className='space-y-4'>
        <div className='text-muted-foreground flex flex-wrap gap-6 text-sm'>
          <span>
            {t('Total budget')}: {formatCurrencyFromUSD(project.total_budget)}
          </span>
          <span>
            {t('Allocated')}: {formatCurrencyFromUSD(project.allocated_total)}
          </span>
          <span
            className={cn(
              remaining >= 0
                ? 'text-emerald-600 dark:text-emerald-400'
                : 'text-destructive'
            )}
          >
            {t('Remaining allocatable')}: {formatCurrencyFromUSD(remaining)}
          </span>
        </div>

        <div className='flex items-center justify-between'>
          <span className='text-sm font-medium'>{t('Plan list')}</span>
          <Button
            size='sm'
            onClick={() => {
              setCurrentPlan(null)
              setOpen('plan-form')
            }}
          >
            <Plus className='h-4 w-4' />
            {t('Create plan')}
          </Button>
        </div>

        <StaticDataTable
          tableClassName='min-w-max'
          data={plans}
          getRowKey={(plan) => plan.id}
          emptyContent={t('No plans yet')}
          emptyClassName='text-muted-foreground py-8'
          columns={[
            { id: 'id', header: t('ID'), cell: (plan) => plan.id },
            {
              id: 'name',
              header: t('Plan name'),
              cellClassName: 'font-medium',
              cell: (plan) => plan.plan_name,
            },
            {
              id: 'start',
              header: t('Start date'),
              cell: (plan) => formatPlanDate(plan.start_date),
            },
            {
              id: 'end',
              header: t('End date'),
              cell: (plan) => formatPlanDate(plan.end_date),
            },
            {
              id: 'status',
              header: t('Status'),
              cell: (plan) => (
                <PlanStatusBadge
                  isActive={plan.is_active}
                  isExpired={plan.is_expired}
                />
              ),
            },
            {
              id: 'allocations',
              header: t('Allocation details'),
              cell: (plan) => {
                const allocations = plan.allocations ?? []
                if (allocations.length === 0) {
                  return <span className='text-muted-foreground'>-</span>
                }
                return (
                  <div className='text-xs'>
                    {allocations.map((allocation) => (
                      <div key={allocation.client_user_id}>
                        {allocation.client_user_id}:{' '}
                        {formatCurrencyFromUSD(allocation.allocated_quota)}
                      </div>
                    ))}
                  </div>
                )
              },
            },
            {
              id: 'allocated-total',
              header: t('Allocated total'),
              cell: (plan) => formatCurrencyFromUSD(plan.allocated_total),
            },
            {
              id: 'actions',
              header: t('Actions'),
              cell: (plan) => (
                <div className='flex gap-1'>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => {
                      setCurrentPlan(plan)
                      setOpen('allocations')
                    }}
                  >
                    {t('Allocate')}
                  </Button>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => {
                      setCurrentPlan(plan)
                      setOpen('plan-form')
                    }}
                  >
                    {t('Edit')}
                  </Button>
                  <Button
                    variant='ghost'
                    size='sm'
                    className='text-destructive'
                    onClick={() => setDeleteTarget(plan)}
                  >
                    {t('Delete')}
                  </Button>
                </div>
              ),
            },
          ]}
        />
      </div>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(isOpen) => !isOpen && setDeleteTarget(null)}
        title={t('Delete plan')}
        desc={t('Delete this plan and all of its allocations?')}
        destructive
        handleConfirm={() => {
          if (deleteTarget) void handleDelete(deleteTarget)
        }}
      />
    </Dialog>
  )
}

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
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { quotaUnitsToDollars } from '@/lib/format'
import { cn } from '@/lib/utils'

import { setActivePlan, updateProjectStatus } from '../api'
import { PROJECT_STATUS, formatPlanDate, todayPlanDate } from '../constants'
import type { Project } from '../types'
import { PlanAllocationsCell } from './plan-allocations-cell'
import { useProjectBudget } from './project-budget-provider'

const NO_ACTIVE_PLAN = '0'

export function useProjectColumns(): ColumnDef<Project>[] {
  const { t } = useTranslation()
  const { setOpen, setCurrentProject, triggerRefresh } = useProjectBudget()

  const handleStatusToggle = async (project: Project) => {
    const nextStatus =
      project.status === PROJECT_STATUS.ENABLED
        ? PROJECT_STATUS.PAUSED
        : PROJECT_STATUS.ENABLED
    const res = await updateProjectStatus(project.id, nextStatus)
    if (!res.success) {
      toast.error(res.message || t('Operation failed'))
      return
    }
    toast.success(t('Status updated'))
    triggerRefresh()
  }

  const handleActivePlanChange = async (project: Project, planId: number) => {
    const res = await setActivePlan(project.id, planId)
    if (!res.success) {
      toast.error(res.message || t('Operation failed'))
      return
    }
    toast.success(t('Switched successfully'))
    triggerRefresh()
  }

  return [
    {
      accessorKey: 'id',
      header: t('ID'),
      meta: { mobileHidden: true },
      size: 70,
    },
    {
      accessorKey: 'project_name',
      header: t('Project name'),
      meta: { mobileTitle: true },
      cell: ({ row }) => (
        <span className='font-medium'>{row.original.project_name}</span>
      ),
      size: 140,
    },
    {
      accessorKey: 'total_budget',
      header: t('Total budget'),
      cell: ({ row }) => formatCurrencyFromUSD(row.original.total_budget),
      size: 110,
    },
    {
      accessorKey: 'quota',
      header: t('Consumed'),
      cell: ({ row }) =>
        formatCurrencyFromUSD(quotaUnitsToDollars(row.original.quota ?? 0), {
          digitsLarge: 6,
          digitsSmall: 6,
        }),
      size: 120,
    },
    {
      id: 'plans',
      header: t('Allocated'),
      enableSorting: false,
      cell: ({ row }) => <PlanAllocationsCell plans={row.original.plans} />,
      size: 280,
    },
    {
      id: 'remaining_budget',
      header: t('Remaining allocatable'),
      cell: ({ row }) => {
        const remaining =
          (row.original.total_budget || 0) - (row.original.allocated_total || 0)
        return (
          <span
            className={cn(
              remaining > 0 && 'text-emerald-600 dark:text-emerald-400',
              remaining < 0 && 'text-destructive'
            )}
          >
            {formatCurrencyFromUSD(remaining)}
          </span>
        )
      },
      size: 140,
    },
    {
      accessorKey: 'status',
      header: t('Status'),
      cell: ({ row }) => {
        const isEnabled = row.original.status === PROJECT_STATUS.ENABLED
        return (
          <div className='flex items-center gap-2'>
            <Badge variant={isEnabled ? 'default' : 'secondary'}>
              {isEnabled ? t('Enabled') : t('Paused')}
            </Badge>
            <Switch
              checked={isEnabled}
              onCheckedChange={() => void handleStatusToggle(row.original)}
              aria-label={t('Status')}
            />
          </div>
        )
      },
      size: 150,
    },
    {
      id: 'active_plan',
      header: t('Active plan'),
      enableSorting: false,
      cell: ({ row }) => {
        // Only plans that have not ended can be activated.
        const today = todayPlanDate()
        const selectablePlans = (row.original.plans ?? []).filter(
          (plan) => plan.end_date >= today
        )
        const items = [
          { value: NO_ACTIVE_PLAN, label: t('Not active') },
          ...selectablePlans.map((plan) => ({
            value: String(plan.plan_id),
            label: `${plan.plan_name} (${formatPlanDate(plan.start_date)}~${formatPlanDate(plan.end_date)})`,
          })),
        ]
        return (
          <Select
            items={items}
            value={String(row.original.active_plan_id || 0)}
            onValueChange={(value) =>
              void handleActivePlanChange(
                row.original,
                Number.parseInt(value ?? NO_ACTIVE_PLAN, 10)
              )
            }
          >
            <SelectTrigger className='h-8 w-full text-xs'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NO_ACTIVE_PLAN}>{t('Not active')}</SelectItem>
              {selectablePlans.map((plan) => (
                <SelectItem key={plan.plan_id} value={String(plan.plan_id)}>
                  {plan.plan_name} ({formatPlanDate(plan.start_date)}~
                  {formatPlanDate(plan.end_date)})
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )
      },
      size: 200,
    },
    {
      id: 'actions',
      header: t('Actions'),
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <div className='flex gap-1'>
          <Button
            variant='outline'
            size='sm'
            onClick={() => {
              setCurrentProject(row.original)
              setOpen('mutate')
            }}
          >
            {t('Edit')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            onClick={() => {
              setCurrentProject(row.original)
              setOpen('plans')
            }}
          >
            {t('Plans')}
          </Button>
        </div>
      ),
      size: 170,
    },
  ]
}

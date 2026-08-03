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
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { formatCurrencyFromUSD } from '@/lib/currency'

import { formatPlanDate } from '../constants'
import type { ProjectPlanSummary } from '../types'

type PlanAllocationsCellProps = {
  plans?: ProjectPlanSummary[]
}

/**
 * Per-plan allocation breakdown inside the project row. The active plan is
 * expanded by default and inactive ones start collapsed, so a project with a
 * long history of plans stays readable.
 */
export function PlanAllocationsCell({ plans }: PlanAllocationsCellProps) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState<Record<number, boolean>>({})

  const visiblePlans = (plans ?? []).filter(
    (plan) => (plan.allocations ?? []).length > 0
  )
  if (visiblePlans.length === 0) {
    return <span className='text-muted-foreground'>-</span>
  }

  return (
    <div className='space-y-2'>
      {visiblePlans.map((plan) => {
        const allocations = plan.allocations ?? []
        const isOpen = expanded[plan.plan_id] ?? plan.is_active
        return (
          <div key={plan.plan_id} className='text-xs'>
            <button
              type='button'
              className='flex items-center gap-1'
              onClick={() =>
                setExpanded((prev) => ({
                  ...prev,
                  [plan.plan_id]: !isOpen,
                }))
              }
              aria-expanded={isOpen}
            >
              {isOpen ? (
                <ChevronDown className='h-3 w-3' />
              ) : (
                <ChevronRight className='h-3 w-3' />
              )}
              <Badge variant={plan.is_active ? 'default' : 'secondary'}>
                {plan.plan_name}
              </Badge>
              <span className='text-muted-foreground'>
                {formatPlanDate(plan.start_date)}~{formatPlanDate(plan.end_date)}
              </span>
              {!isOpen && (
                <span className='text-muted-foreground'>
                  {t('({{count}} items)', { count: allocations.length })}
                </span>
              )}
            </button>
            {isOpen && (
              <div className='ps-5'>
                {allocations.map((allocation) => (
                  <div key={allocation.client_user_id}>
                    {allocation.client_user_id}:{' '}
                    {formatCurrencyFromUSD(allocation.allocated_quota)}
                  </div>
                ))}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

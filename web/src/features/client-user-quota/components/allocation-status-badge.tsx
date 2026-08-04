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
import { todayPlanDate } from '@/lib/format'

import type { ProjectAllocation } from '../types'

/**
 * Why an allocation is (not) currently effective.
 *
 * The backend sends three independent flags rather than one status, so the
 * order here matters: an allocation can be both "active plan" and "out of date
 * range", and the user needs the more specific reason.
 */
export function AllocationStatusBadge({
  allocation,
}: {
  allocation: ProjectAllocation
}) {
  const { t } = useTranslation()

  if (allocation.is_current_effective) {
    return <Badge variant='default'>{t('Currently effective')}</Badge>
  }
  if (allocation.project_status === 2) {
    return <Badge variant='warning'>{t('Project paused')}</Badge>
  }
  if (allocation.is_active_plan && !allocation.is_in_date_range) {
    return allocation.start_date > todayPlanDate() ? (
      <Badge variant='secondary'>{t('Not started')}</Badge>
    ) : (
      <Badge variant='destructive'>{t('Expired')}</Badge>
    )
  }
  return <Badge variant='outline'>{t('Historical plan')}</Badge>
}

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

import { PROJECT_STATUS, formatPlanDate, todayPlanDate } from '../constants'
import type { Project } from '../types'

/**
 * Why the selected plan is (or is not) taking effect.
 *
 * Picking a plan is not enough — the project must be enabled and today must
 * fall inside the plan's date range. Showing only the plan name would leave a
 * paused project or an expired plan looking healthy.
 */
export function ActivePlanStatusBadge({ project }: { project: Project }) {
  const { t } = useTranslation()

  if (!project.active_plan_id) {
    return <Badge variant='outline'>{t('No plan selected')}</Badge>
  }
  if (project.status !== PROJECT_STATUS.ENABLED) {
    return <Badge variant='warning'>{t('Project paused')}</Badge>
  }
  if (project.active_plan_effective) {
    return <Badge variant='default'>{t('Currently effective')}</Badge>
  }
  return (project.active_plan_start_date ?? '') > todayPlanDate() ? (
    <Badge variant='secondary'>{t('Selected · not started')}</Badge>
  ) : (
    <Badge variant='destructive'>{t('Selected · expired')}</Badge>
  )
}

/** Date range of the selected plan, or a dash when no plan is selected. */
export function ActivePlanPeriod({ project }: { project: Project }) {
  if (!project.active_plan_id) return null
  return (
    <span className='text-muted-foreground text-xs'>
      {formatPlanDate(project.active_plan_start_date)} ~{' '}
      {formatPlanDate(project.active_plan_end_date)}
    </span>
  )
}

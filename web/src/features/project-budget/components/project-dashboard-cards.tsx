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

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { quotaUnitsToDollars } from '@/lib/format'

import { getProjectDashboard } from '../api'

/** Totals across every project the caller can see. */
export function ProjectDashboardCards() {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['project-budget', 'dashboard'],
    queryFn: getProjectDashboard,
  })

  const projects = data?.projects ?? []
  const sum = (pick: (project: (typeof projects)[number]) => number) =>
    projects.reduce((total, project) => total + (pick(project) || 0), 0)

  const cards = [
    { label: t('Total projects'), value: String(projects.length) },
    {
      label: t('Total budget'),
      value: formatCurrencyFromUSD(sum((project) => project.total_budget)),
    },
    {
      label: t('Allocated'),
      value: formatCurrencyFromUSD(sum((project) => project.allocated_total)),
    },
    {
      label: t('Used'),
      value: formatCurrencyFromUSD(
        quotaUnitsToDollars(sum((project) => project.used_total)),
        { digitsLarge: 6, digitsSmall: 6 }
      ),
    },
  ]

  return (
    <div className='grid grid-cols-2 gap-4 md:grid-cols-4'>
      {cards.map((card) => (
        <Card key={card.label}>
          <CardHeader>
            <CardTitle className='text-muted-foreground text-sm font-normal'>
              {card.label}
            </CardTitle>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <Skeleton className='h-6 w-24' />
            ) : (
              <span className='text-lg font-semibold'>{card.value}</span>
            )}
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

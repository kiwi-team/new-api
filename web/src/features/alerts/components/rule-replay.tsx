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

import { StaticDataTable } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { toIntlLocale } from '@/i18n/languages'

import { alertLabel } from '../lib/labels'
import { formatAlertMetricValue } from '../lib/rule-form'

export type ReplayPoint = {
  target: { type: string; id: number; name: string; model: string }
  at: number
  measurement: {
    status: string
    reason?: string
    value: string
    samples: string
  }
  matched: boolean
}
export function ReplayResults(props: {
  points: ReplayPoint[]
  metric: string
}) {
  const { t, i18n } = useTranslation()
  const date = new Intl.DateTimeFormat(toIntlLocale(i18n.language), {
    dateStyle: 'short',
    timeStyle: 'short',
  })
  return (
    <section className='mt-4 space-y-2'>
      <h3 className='font-medium'>{t('Historical threshold preview')}</h3>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Replay reads recorded facts. It does not send notifications or execute actions.'
        )}
      </p>
      <StaticDataTable
        data={props.points}
        getRowKey={(point) =>
          `${point.target.type}:${point.target.id}:${point.target.model}:${point.at}`
        }
        columns={[
          {
            id: 'target',
            header: t('Monitored object'),
            cell: (point) => point.target.name,
          },
          {
            id: 'at',
            header: t('Time'),
            cell: (point) => date.format(point.at),
          },
          {
            id: 'value',
            header: t('Value'),
            cell: (point) =>
              formatAlertMetricValue(
                props.metric,
                point.measurement.value,
                toIntlLocale(i18n.resolvedLanguage || i18n.language),
                point.measurement
              ),
          },
          {
            id: 'samples',
            header: t('Samples'),
            cell: (point) => point.measurement.samples,
          },
          {
            id: 'coverage',
            header: t('Data coverage'),
            cell: (point) => (
              <>
                <Badge variant='outline'>
                  {alertLabel(point.measurement.status, t)}
                </Badge>
                {point.measurement.reason && (
                  <p className='text-muted-foreground text-xs'>
                    {alertLabel(point.measurement.reason, t)}
                  </p>
                )}
              </>
            ),
          },
          {
            id: 'matched',
            header: t('Threshold matched'),
            cell: (point) => t(point.matched ? 'Yes' : 'No'),
          },
        ]}
      />
    </section>
  )
}

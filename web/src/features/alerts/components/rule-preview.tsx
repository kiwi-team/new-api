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
import type { Preview } from '../types'

export function PreviewResults(props: { preview: Preview }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  return (
    <section className='mt-4 space-y-2' aria-label={t('Preview')}>
      <h3 className='font-medium'>{t('Preview')}</h3>
      <StaticDataTable
        data={props.preview.targets}
        getRowKey={(row) =>
          `${row.target.type}:${row.target.id}:${row.target.model}`
        }
        columns={[
          {
            id: 'target',
            header: t('Monitored object'),
            cell: (row) => <span className='break-all'>{row.target.name}</span>,
          },
          {
            id: 'value',
            header: t('Value'),
            cell: (row) =>
              formatAlertMetricValue(
                props.preview.config.condition.metric,
                row.measurement.value,
                locale,
                row.measurement
              ),
          },
          {
            id: 'coverage',
            header: t('Data coverage'),
            cell: (row) => (
              <>
                <Badge variant='outline'>
                  {alertLabel(row.measurement.status, t)}
                </Badge>
                <p className='text-muted-foreground text-xs'>
                  {row.measurement.reason
                    ? alertLabel(row.measurement.reason, t)
                    : null}
                </p>
              </>
            ),
          },
          {
            id: 'impact',
            header: t('Action impact'),
            cell: (row) =>
              row.impact ? (
                <>
                  <Badge variant='outline'>
                    {alertLabel(row.impact.status, t)}
                  </Badge>
                  {row.impact.reason && (
                    <p>{alertLabel(row.impact.reason, t)}</p>
                  )}
                  {row.impact.domains.map((domain) => (
                    <p
                      className='text-xs'
                      key={`${domain.group}:${domain.model}`}
                    >
                      {domain.group} / {domain.model}:{' '}
                      {domain.alternatives.join(', ') || t('None')}
                    </p>
                  ))}
                </>
              ) : (
                t('Notify only')
              ),
          },
        ]}
        emptyContent={t('No matching objects')}
      />
    </section>
  )
}

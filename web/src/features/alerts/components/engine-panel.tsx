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
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { alertRequest } from '../api'
import { alertLabel } from '../lib/labels'
import type { EngineHealth } from '../types'

export function EnginePanel() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const query = useQuery({
    queryKey: ['alerts', 'engine-health'],
    queryFn: () => alertRequest<EngineHealth>('engine-health'),
    refetchInterval: 10000,
  })
  if (query.isPending) return <LoadingState />
  if (query.isError) return <ErrorState onRetry={() => void query.refetch()} />
  const health = query.data
  return (
    <section className='space-y-4'>
      <div className='flex flex-wrap gap-3'>
        <Badge variant='outline'>
          {t(health.enabled ? 'Enabled' : 'Disabled')}
        </Badge>
        <span>
          {t('Pending delivery jobs')}: {health.job_backlog}
        </span>
        <Badge variant='outline'>
          {t(
            health.actions_enabled
              ? 'Automatic actions enabled'
              : 'Automatic actions disabled'
          )}
        </Badge>
      </div>
      <p className='text-sm'>
        {t('Alert input mode')}: {alertLabel(health.receive_mode, t)} ·{' '}
        {t('Rules with evaluation errors')}:{' '}
        {formatNumber(health.failed_rules, locale)}
      </p>
      {!!health.uncovered_nodes?.length && (
        <p>
          {t('Uncovered nodes')}: {health.uncovered_nodes.join(', ')}
        </p>
      )}
      <StaticDataTable
        data={health.sources}
        getRowKey={(source) => source.id}
        columns={[
          { id: 'node', header: t('Node'), cell: (source) => source.node_id },
          {
            id: 'coverage',
            header: t('Data coverage'),
            cell: (source) => (
              <>
                <Badge variant='outline'>
                  {alertLabel(
                    source.healthy &&
                      source.updated_at <= Date.now() + 5000 &&
                      Date.now() - source.updated_at <
                        (source.kind === 'producer' ? 15000 : 60000) &&
                      (source.kind !== 'producer' ||
                        (source.protocol_version >= 2 &&
                          Date.now() - source.captured_at < 15000 &&
                          source.captured_at <= Date.now() + 5000 &&
                          source.accepted_through >= source.offered_sequence))
                      ? 'complete'
                      : 'stale',
                    t
                  )}
                </Badge>
                {source.last_error && (
                  <p className='text-muted-foreground text-xs'>
                    {alertLabel(source.last_error, t)}
                  </p>
                )}
              </>
            ),
          },
          {
            id: 'accepted',
            header: t('Accepted / offered'),
            cell: (source) =>
              `${source.accepted_through} / ${source.offered_sequence}`,
          },
        ]}
        emptyContent={t('No covered producers')}
      />
    </section>
  )
}

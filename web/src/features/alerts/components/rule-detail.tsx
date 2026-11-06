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
import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'

import { alertRequest } from '../api'
import { alertLabel } from '../lib/labels'
import type { AlertRule } from '../types'
import { ReportQuery } from './report-query'

export function RuleDetailDialog(props: {
  rule: AlertRule
  onClose: () => void
}) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['alerts', 'rule', props.rule.id],
    queryFn: () => alertRequest<AlertRule>(`rules/${props.rule.id}`),
    refetchInterval: 10000,
  })
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={props.rule.config.name}
      description={t('Data coverage')}
      contentClassName='sm:max-w-4xl'
    >
      {query.isPending && <LoadingState />}
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      {query.data && (
        <div className='space-y-4 text-sm'>
          <p>
            {alertLabel(query.data.config.condition.metric, t)} /{' '}
            {alertLabel(
              query.data.config.condition.window.type ||
                query.data.config.condition.deadline_kind,
              t
            )}
          </p>
          <StaticDataTable
            data={query.data.targets ?? []}
            getRowKey={(target) =>
              `${target.type}:${target.id}:${target.model}`
            }
            columns={[
              {
                id: 'name',
                header: t('Monitored object'),
                cell: (target) => target.name,
              },
              {
                id: 'scope',
                header: t('Scope'),
                cell: (target) => alertLabel(target.type, t),
              },
            ]}
          />
          <StaticDataTable
            data={query.data.states ?? []}
            getRowKey={(state) => state.id}
            columns={[
              {
                id: 'name',
                header: t('Monitored object'),
                cell: (state) =>
                  state.target_name ||
                  state.model_key ||
                  props.rule.config.name,
              },
              {
                id: 'status',
                header: t('Status'),
                cell: (state) => alertLabel(state.status, t),
              },
              {
                id: 'coverage',
                header: t('Data coverage'),
                cell: (state) => (
                  <Badge variant='outline'>
                    {alertLabel(state.data_status || 'no_data', t)}
                  </Badge>
                ),
              },
              {
                id: 'value',
                header: t('Value'),
                cell: (state) => state.last_value,
              },
              {
                id: 'samples',
                header: t('Valid samples'),
                cell: (state) => state.last_count,
              },
            ]}
          />
          {query.data.config.trigger.mode === 'schedule_report' && (
            <ReportQuery rule={query.data} />
          )}
        </div>
      )}
    </Dialog>
  )
}

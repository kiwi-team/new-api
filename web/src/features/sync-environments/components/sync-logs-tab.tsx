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
import { StatusBadge } from '@/components/status-badge'
import { formatTimestampToDate } from '@/lib/format'

import { getSyncLogs } from '../api'
import { SYNC_LOG_STATUS } from '../types'

/** One page of history is enough context; older runs are rarely inspected. */
const PAGE_SIZE = 50

export function SyncLogsTab() {
  const { t } = useTranslation()

  const { data } = useQuery({
    queryKey: ['sync-environments', 'logs'],
    queryFn: () => getSyncLogs({ page: 1, page_size: PAGE_SIZE }),
  })

  return (
    <StaticDataTable
      tableClassName='min-w-max'
      data={data?.items ?? []}
      getRowKey={(log) => log.id}
      emptyContent={t('No sync history yet')}
      emptyClassName='text-muted-foreground py-8'
      columns={[
        { id: 'id', header: t('ID'), cell: (log) => log.id },
        {
          id: 'type',
          header: t('Sync type'),
          cell: (log) =>
            log.sync_type === 'model_price'
              ? t('Model prices')
              : t('Channels'),
        },
        {
          id: 'environment',
          header: t('Target environment'),
          cellClassName: 'font-medium',
          cell: (log) => log.environment_name || '-',
        },
        {
          id: 'summary',
          header: t('Data summary'),
          cell: (log) => (
            <span className='block max-w-md truncate'>
              {log.data_summary || '-'}
            </span>
          ),
        },
        {
          id: 'status',
          header: t('Status'),
          cell: (log) => (
            <StatusBadge
              label={
                log.status === SYNC_LOG_STATUS.SUCCESS
                  ? t('Succeeded')
                  : t('Failed')
              }
              variant={
                log.status === SYNC_LOG_STATUS.SUCCESS ? 'success' : 'danger'
              }
              size='sm'
              copyable={false}
            />
          ),
        },
        {
          id: 'error',
          header: t('Error message'),
          cellClassName: 'text-destructive',
          cell: (log) => (
            <span className='block max-w-md truncate'>
              {log.error_message || '-'}
            </span>
          ),
        },
        {
          id: 'time',
          header: t('Sync time'),
          cell: (log) =>
            log.created_time ? formatTimestampToDate(log.created_time) : '-',
        },
      ]}
    />
  )
}

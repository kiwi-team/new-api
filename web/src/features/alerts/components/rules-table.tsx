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
import { Button } from '@/components/ui/button'

import { alertLabel } from '../lib/labels'
import type { AlertRule } from '../types'

type Props = {
  rules: AlertRule[]
  canManage: (rule: AlertRule) => boolean
  onView: (rule: AlertRule) => void
  onEdit: (rule: AlertRule) => void
  onPause: (rule: AlertRule) => void
  onDelete: (rule: AlertRule) => void
}
export function RulesTable(props: Props) {
  const { t } = useTranslation()
  return (
    <StaticDataTable
      data={props.rules}
      getRowKey={(row) => row.id}
      columns={[
        {
          id: 'name',
          header: t('Rule'),
          cell: (row) => (
            <div className='max-w-72 font-medium break-words'>
              {row.config.name}
              <p className='text-muted-foreground text-xs'>
                {alertLabel(row.config.management_mode, t)}
              </p>
            </div>
          ),
        },
        {
          id: 'metric',
          header: t('Metric'),
          cell: (row) => (
            <>
              {alertLabel(row.config.condition.metric, t)}
              <p className='text-muted-foreground text-xs'>
                {alertLabel(row.config.scope.type, t)} /{' '}
                {alertLabel(
                  row.config.condition.window.type ||
                    row.config.condition.deadline_kind,
                  t
                )}
              </p>
            </>
          ),
        },
        {
          id: 'status',
          header: t('Status'),
          cell: (row) => (
            <>
              <Badge variant='outline'>
                {t(row.config.enabled ? 'Enabled' : 'Paused')}
              </Badge>
              <Badge variant='outline'>{row.config.severity}</Badge>
              <p className='text-muted-foreground text-xs'>
                {alertLabel(row.runtime?.data_status || 'no_data', t)}
                {row.last_evaluation_error && (
                  <span className='block'>
                    {alertLabel(row.last_evaluation_error, t)}
                  </span>
                )}
              </p>
            </>
          ),
        },
        {
          id: 'action',
          header: t('Actions'),
          cell: (row) => (
            <div className='flex flex-wrap gap-2'>
              <Button
                size='sm'
                variant='outline'
                onClick={() => props.onView(row)}
              >
                {t('Details')}
              </Button>
              <Button
                size='sm'
                variant='outline'
                onClick={() => props.onEdit(row)}
                disabled={!props.canManage(row) || row.config_valid === false}
              >
                {t('Edit')}
              </Button>
              {props.canManage(row) && row.config.enabled && (
                <Button
                  size='sm'
                  variant='outline'
                  onClick={() => props.onPause(row)}
                >
                  {t('Pause')}
                </Button>
              )}
              {props.canManage(row) &&
                !row.config.enabled &&
                row.config_valid !== false && (
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() =>
                      props.onEdit({
                        ...row,
                        config: { ...row.config, enabled: true },
                      })
                    }
                  >
                    {t('Resume')}
                  </Button>
                )}
              {props.canManage(row) && (
                <Button
                  size='sm'
                  variant='ghost'
                  onClick={() => props.onDelete(row)}
                >
                  {t('Delete')}
                </Button>
              )}
            </div>
          ),
        },
      ]}
      emptyContent={t('No alert rules')}
    />
  )
}

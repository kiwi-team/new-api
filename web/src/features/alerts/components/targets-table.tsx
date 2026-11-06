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

import type { AlertTarget } from '../types'

type Props = {
  targets: AlertTarget[]
  canManage: (target: AlertTarget) => boolean
  onEdit: (target: AlertTarget) => void
  onTest: (target: AlertTarget) => void
  onDelete: (target: AlertTarget) => void
}
export function TargetsTable(props: Props) {
  const { t } = useTranslation()
  return (
    <StaticDataTable
      data={props.targets}
      getRowKey={(row) => row.id}
      columns={[
        { id: 'name', header: t('Name'), cell: (row) => row.name },
        { id: 'type', header: t('Type'), cell: (row) => row.type },
        {
          id: 'address',
          header: t('Destination'),
          cell: (row) => <span className='break-all'>{row.address_hint}</span>,
        },
        {
          id: 'status',
          header: t('Status'),
          cell: (row) => (
            <Badge variant='outline'>
              {t(row.enabled ? 'Enabled' : 'Disabled')}
            </Badge>
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
                disabled={!props.canManage(row)}
                onClick={() => props.onEdit(row)}
              >
                {t('Edit')}
              </Button>
              <Button
                size='sm'
                variant='outline'
                disabled={!row.enabled || !props.canManage(row)}
                onClick={() => props.onTest(row)}
              >
                {t('Test')}
              </Button>
              <Button
                size='sm'
                variant='ghost'
                disabled={!props.canManage(row)}
                onClick={() => props.onDelete(row)}
              >
                {t('Delete')}
              </Button>
            </div>
          ),
        },
      ]}
      emptyContent={t('No notification targets')}
    />
  )
}

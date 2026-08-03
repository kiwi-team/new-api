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
import type { Row } from '@tanstack/react-table'
import { Edit, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import type { ClientUserQuota } from '../types'
import { useClientUserQuota } from './client-user-quota-provider'

type ClientUserQuotaRowActionsProps = {
  row: Row<ClientUserQuota>
}

export function ClientUserQuotaRowActions({
  row,
}: ClientUserQuotaRowActionsProps) {
  const { t } = useTranslation()
  const { setOpen, setCurrentRow } = useClientUserQuota()

  const openDialog = (dialog: 'mutate' | 'delete') => {
    setCurrentRow(row.original)
    setOpen(dialog)
  }

  return (
    <div className='-ml-1.5 flex items-center gap-1'>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon'
              aria-label={t('Edit')}
              onClick={() => openDialog('mutate')}
            />
          }
        >
          <Edit className='h-4 w-4' />
        </TooltipTrigger>
        <TooltipContent>{t('Edit')}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon'
              className='text-destructive'
              aria-label={t('Delete')}
              onClick={() => openDialog('delete')}
            />
          }
        >
          <Trash2 className='h-4 w-4' />
        </TooltipTrigger>
        <TooltipContent>{t('Delete')}</TooltipContent>
      </Tooltip>
    </div>
  )
}

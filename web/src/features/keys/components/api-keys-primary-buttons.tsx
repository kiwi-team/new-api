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
import { ListPlus, MoreHorizontal, Plus } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { AdminCsvMenuItems } from '@/components/admin-csv-menu-items'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useApiKeys } from './api-keys-provider'
import { BatchAppendModelsDialog } from './batch-append-models-dialog'

export function ApiKeysPrimaryButtons() {
  const { t } = useTranslation()
  const { setOpen, triggerRefresh } = useApiKeys()
  const [showAppendModels, setShowAppendModels] = useState(false)
  // CSV import/export is root-only on the backend.
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )

  return (
    <div className='flex gap-2'>
      {isRoot && (
        <DropdownMenu modal={false}>
          <DropdownMenuTrigger
            render={<Button variant='outline' size='sm' />}
            aria-label={t('Batch Operations')}
          >
            <MoreHorizontal className='h-4 w-4' />
          </DropdownMenuTrigger>
          <DropdownMenuContent align='end' className='w-56'>
            <AdminCsvMenuItems entity='tokens' onImported={triggerRefresh} />
            <DropdownMenuSeparator />
            {/* Group-scoped, so it lives here rather than in the selection
                toolbar. */}
            <DropdownMenuItem onClick={() => setShowAppendModels(true)}>
              <ListPlus className='h-4 w-4' />
              {t('Batch append models')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      )}
      <Button size='sm' onClick={() => setOpen('create')}>
        <Plus className='h-4 w-4' />
        {t('Create API Key')}
      </Button>

      {showAppendModels && (
        <BatchAppendModelsDialog
          open
          onOpenChange={setShowAppendModels}
          onDone={triggerRefresh}
        />
      )}
    </div>
  )
}

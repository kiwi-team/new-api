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
import { Download, Upload } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  DropdownMenuItem,
  DropdownMenuShortcut,
} from '@/components/ui/dropdown-menu'
import {
  type AdminCsvEntity,
  exportAdminCsv,
  importAdminCsv,
} from '@/lib/admin-csv'

type AdminCsvMenuItemsProps = {
  entity: AdminCsvEntity
  /** Called after a successful import so the list can refetch. */
  onImported?: () => void
}

/**
 * Root-only CSV import/export entries, rendered inside an existing dropdown.
 * The hidden file input lives here so both menu items stay self-contained.
 */
export function AdminCsvMenuItems({
  entity,
  onImported,
}: AdminCsvMenuItemsProps) {
  const { t } = useTranslation()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [isBusy, setIsBusy] = useState(false)

  const handleExport = async () => {
    setIsBusy(true)
    try {
      await exportAdminCsv(entity)
    } catch {
      toast.error(t('Export failed'))
    } finally {
      setIsBusy(false)
    }
  }

  const handleImport = async (file: File) => {
    setIsBusy(true)
    try {
      const result = await importAdminCsv(entity, file)
      if (!result.success) {
        toast.error(result.message || t('Import failed'))
        return
      }
      toast.success(t('Imported successfully'))
      onImported?.()
    } catch {
      toast.error(t('Import failed'))
    } finally {
      setIsBusy(false)
    }
  }

  return (
    <>
      <DropdownMenuItem disabled={isBusy} onClick={() => void handleExport()}>
        {t('Export CSV')}
        <DropdownMenuShortcut>
          <Download className='h-4 w-4' />
        </DropdownMenuShortcut>
      </DropdownMenuItem>

      <DropdownMenuItem
        disabled={isBusy}
        onClick={(event) => {
          // Keep the menu from closing before the file dialog opens.
          event.preventDefault()
          fileInputRef.current?.click()
        }}
      >
        {t('Import CSV')}
        <DropdownMenuShortcut>
          <Upload className='h-4 w-4' />
        </DropdownMenuShortcut>
      </DropdownMenuItem>

      <input
        ref={fileInputRef}
        type='file'
        accept='.csv'
        className='hidden'
        onChange={(event) => {
          const file = event.target.files?.[0]
          event.target.value = ''
          if (file) void handleImport(file)
        }}
      />
    </>
  )
}

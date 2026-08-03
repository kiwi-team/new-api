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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'

import { batchImportSettlementConfigs } from '../api'
import { BATCH_IMPORT_EXAMPLE } from '../constants'

type BatchImportDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Receives the imported `user_id` so the page can jump to that customer. */
  onImported: (userId?: number) => void
}

/** Import a whole customer's settlement overrides from a JSON payload. */
export function BatchImportDialog({
  open,
  onOpenChange,
  onImported,
}: BatchImportDialogProps) {
  const { t } = useTranslation()
  const [json, setJson] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleImport = async () => {
    if (!json.trim()) {
      toast.error(t('Please enter the configuration data in JSON format'))
      return
    }
    let parsed: { user_id?: number }
    try {
      parsed = JSON.parse(json)
    } catch {
      toast.error(t('Invalid JSON format'))
      return
    }
    setIsSubmitting(true)
    try {
      const res = await batchImportSettlementConfigs(parsed)
      if (!res.success) {
        toast.error(res.message || t('Operation failed'))
        return
      }
      toast.success(t('Imported successfully'))
      setJson('')
      onOpenChange(false)
      onImported(parsed.user_id)
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Batch import')}
      contentClassName='sm:max-w-2xl'
      contentHeight='auto'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={() => void handleImport()}
            disabled={isSubmitting}
          >
            {t('Import')}
          </Button>
        </>
      }
    >
      <div className='space-y-2'>
        <Textarea
          rows={12}
          placeholder={t('Please enter the configuration data in JSON format')}
          value={json}
          onChange={(event) => setJson(event.target.value)}
          className='font-mono text-xs'
        />
        <pre className='text-muted-foreground overflow-x-auto text-xs'>
          {BATCH_IMPORT_EXAMPLE}
        </pre>
      </div>
    </Dialog>
  )
}

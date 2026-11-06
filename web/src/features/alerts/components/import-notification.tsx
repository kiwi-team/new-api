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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

import { alertRequest } from '../api'

export function ImportNotification(props: { disabled: boolean }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [open, setOpen] = useState(false)
  const [feishu, setFeishu] = useState(false)
  const imported = useMutation({
    mutationFn: () =>
      alertRequest('targets/import-notification', 'POST', { feishu }),
    onSuccess: () => {
      setOpen(false)
      void client.invalidateQueries({ queryKey: ['alerts'] })
    },
  })
  return (
    <>
      <Button
        variant='outline'
        disabled={props.disabled}
        onClick={() => {
          setFeishu(false)
          setOpen(true)
        }}
      >
        {t('Import profile destination')}
      </Button>
      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={t('Import profile destination')}
        desc={t(
          'Copy your current notification destination. Existing subscriptions continue using their current executor.'
        )}
        isLoading={imported.isPending}
        handleConfirm={() => imported.mutate()}
      >
        <div className='flex items-center gap-2'>
          <Switch
            id='alert-import-feishu'
            checked={feishu}
            onCheckedChange={setFeishu}
          />
          <Label htmlFor='alert-import-feishu'>
            {t('The existing webhook is a Feishu bot')}
          </Label>
        </div>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Generic webhooks keep the v1 format unless explicitly identified as Feishu.'
          )}
        </p>
        {imported.isError && <p role='alert'>{imported.error.message}</p>}
      </ConfirmDialog>
    </>
  )
}

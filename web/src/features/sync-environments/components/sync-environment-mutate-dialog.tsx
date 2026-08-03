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
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

import { createSyncEnvironment, updateSyncEnvironment } from '../api'
import { SYNC_ENVIRONMENT_STATUS, type SyncEnvironment } from '../types'

const FORM_ID = 'sync-environment-form'

type FormValues = {
  name: string
  api_url: string
  root_token: string
  new_api_user: string
  remark: string
  enabled: boolean
}

type SyncEnvironmentMutateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: SyncEnvironment | null
  onSaved: () => void
}

export function SyncEnvironmentMutateDialog({
  open,
  onOpenChange,
  currentRow,
  onSaved,
}: SyncEnvironmentMutateDialogProps) {
  const { t } = useTranslation()
  const isEdit = !!currentRow
  const [isSubmitting, setIsSubmitting] = useState(false)

  const form = useForm<FormValues>({
    defaultValues: {
      name: '',
      api_url: '',
      root_token: '',
      new_api_user: '',
      remark: '',
      enabled: true,
    },
  })

  useEffect(() => {
    if (!open) return
    form.reset({
      name: currentRow?.name ?? '',
      api_url: currentRow?.api_url ?? '',
      // The API never returns the stored token, so editing starts blank and
      // an empty value means "keep the existing one".
      root_token: '',
      new_api_user: currentRow?.new_api_user ?? '',
      remark: currentRow?.remark ?? '',
      enabled: currentRow
        ? currentRow.status === SYNC_ENVIRONMENT_STATUS.ENABLED
        : true,
    })
  }, [currentRow, form, open])

  const handleSubmit = async (values: FormValues) => {
    const token = values.root_token.trim()
    if (!isEdit && !token) {
      form.setError('root_token', { message: t('Root token is required') })
      return
    }
    const payload = {
      name: values.name.trim(),
      api_url: values.api_url.trim().replace(/\/+$/, ''),
      ...(token ? { root_token: token } : {}),
      new_api_user: values.new_api_user.trim(),
      remark: values.remark,
      status: values.enabled
        ? SYNC_ENVIRONMENT_STATUS.ENABLED
        : SYNC_ENVIRONMENT_STATUS.DISABLED,
    }
    setIsSubmitting(true)
    try {
      const res = currentRow
        ? await updateSyncEnvironment(currentRow.id, payload)
        : await createSyncEnvironment(payload)
      if (!res.success) {
        toast.error(res.message || t('Operation failed'))
        return
      }
      toast.success(
        isEdit ? t('Updated successfully') : t('Created successfully')
      )
      onOpenChange(false)
      onSaved()
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={isEdit ? t('Edit environment') : t('Add environment')}
      contentClassName='sm:max-w-lg'
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
          <Button type='submit' form={FORM_ID} disabled={isSubmitting}>
            {t('Save')}
          </Button>
        </>
      }
    >
      <form
        id={FORM_ID}
        onSubmit={form.handleSubmit(handleSubmit)}
        className='space-y-4'
      >
        <div className='grid gap-1.5'>
          <Label htmlFor='env-name'>{t('Environment name')} *</Label>
          <Input
            id='env-name'
            placeholder={t('Staging')}
            {...form.register('name', { required: true })}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='env-url'>{t('API URL')} *</Label>
          <Input
            id='env-url'
            placeholder='https://example.com'
            {...form.register('api_url', { required: true })}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='env-token'>
            {t('Root token')} {isEdit ? '' : '*'}
          </Label>
          <Input
            id='env-token'
            type='password'
            autoComplete='new-password'
            placeholder={
              isEdit ? t('Leave empty to keep the current token') : undefined
            }
            {...form.register('root_token')}
          />
          {form.formState.errors.root_token?.message && (
            <p className='text-destructive text-xs'>
              {form.formState.errors.root_token.message}
            </p>
          )}
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='env-user'>{t('New-API user ID')} *</Label>
          <Input
            id='env-user'
            placeholder='1'
            {...form.register('new_api_user', { required: true })}
          />
          <p className='text-muted-foreground text-xs'>
            {t('Sent as the New-API-User header on the target deployment')}
          </p>
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='env-remark'>{t('Remark')}</Label>
          <Input id='env-remark' {...form.register('remark')} />
        </div>
        <div className='flex items-center justify-between rounded-lg border p-3'>
          <Label htmlFor='env-enabled'>{t('Enabled')}</Label>
          <Switch
            id='env-enabled'
            checked={form.watch('enabled')}
            onCheckedChange={(checked) => form.setValue('enabled', checked)}
          />
        </div>
      </form>
    </Dialog>
  )
}

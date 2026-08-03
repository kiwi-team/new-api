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

import { DateTimePicker } from '@/components/datetime-picker'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { createClientUserQuota, updateClientUserQuota } from '../api'
import type { ClientUserQuota } from '../types'

const FORM_ID = 'client-user-quota-form'

type FormValues = {
  client_user_id: string
  client_name: string
  fixed_quota: string
  temp_quota: string
  remark: string
}

/**
 * Temporary budgets default to expiring at the last second of the current
 * month, so the budget covers the whole month rather than spilling into the
 * first instant of the next one.
 */
function endOfCurrentMonth(): Date {
  const now = new Date()
  return new Date(now.getFullYear(), now.getMonth() + 1, 0, 23, 59, 59, 0)
}

type ClientUserQuotaMutateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: ClientUserQuota | null
  onSaved: () => void
}

export function ClientUserQuotaMutateDialog({
  open,
  onOpenChange,
  currentRow,
  onSaved,
}: ClientUserQuotaMutateDialogProps) {
  const { t } = useTranslation()
  const isEdit = !!currentRow
  const role = useAuthStore((state) => state.auth.user?.role) ?? ROLE.GUEST
  const canEditClientName = role >= ROLE.ADMIN
  const [expiredAt, setExpiredAt] = useState<Date | undefined>(undefined)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const form = useForm<FormValues>({
    defaultValues: {
      client_user_id: '',
      client_name: '',
      fixed_quota: '0',
      temp_quota: '0',
      remark: '',
    },
  })

  useEffect(() => {
    if (!open) return
    form.reset({
      client_user_id: currentRow?.client_user_id ?? '',
      client_name: currentRow?.client_name ?? '',
      fixed_quota: String(currentRow?.fixed_quota ?? 0),
      temp_quota: String(currentRow?.temp_quota ?? 0),
      remark: currentRow?.remark ?? '',
    })
    if (currentRow?.expired_at) {
      setExpiredAt(new Date(currentRow.expired_at * 1000))
    } else {
      // New records default to the end of the current month; an existing
      // record without an expiry keeps none.
      setExpiredAt(currentRow ? undefined : endOfCurrentMonth())
    }
  }, [currentRow, form, open])

  const handleSubmit = async (values: FormValues) => {
    const payload = {
      client_user_id: values.client_user_id.trim(),
      client_name: canEditClientName ? values.client_name : undefined,
      fixed_quota: Number.parseInt(values.fixed_quota, 10) || 0,
      temp_quota: Number.parseInt(values.temp_quota, 10) || 0,
      remark: values.remark,
      expired_at: expiredAt ? Math.floor(expiredAt.getTime() / 1000) : null,
    }
    setIsSubmitting(true)
    try {
      const res = currentRow
        ? await updateClientUserQuota({ ...payload, id: currentRow.id })
        : await createClientUserQuota(payload)
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
      title={isEdit ? t('Edit budget') : t('Create budget')}
      contentClassName='sm:max-w-md'
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
          <Label htmlFor='cuq-uid'>{t('Client UID')} *</Label>
          <Input
            id='cuq-uid'
            disabled={isEdit}
            {...form.register('client_user_id', { required: true })}
          />
        </div>

        {canEditClientName && (
          <div className='grid gap-1.5'>
            <Label htmlFor='cuq-name'>{t('Client name')}</Label>
            <Input
              id='cuq-name'
              placeholder={t('Customer name (visible to admins only)')}
              {...form.register('client_name')}
            />
          </div>
        )}

        <div className='grid gap-1.5'>
          <Label htmlFor='cuq-fixed'>{t('Monthly fixed budget')}</Label>
          <Input
            id='cuq-fixed'
            type='number'
            min={0}
            {...form.register('fixed_quota')}
          />
        </div>

        <div className='grid gap-1.5'>
          <Label htmlFor='cuq-temp'>{t('Temporary budget')}</Label>
          <Input
            id='cuq-temp'
            type='number'
            min={0}
            {...form.register('temp_quota')}
          />
        </div>

        <div className='grid gap-1.5'>
          <Label>{t('Temporary budget expires at')}</Label>
          <DateTimePicker value={expiredAt} onChange={setExpiredAt} />
        </div>

        <div className='grid gap-1.5'>
          <Label htmlFor='cuq-remark'>{t('Remark')}</Label>
          <Input id='cuq-remark' {...form.register('remark')} />
        </div>
      </form>
    </Dialog>
  )
}

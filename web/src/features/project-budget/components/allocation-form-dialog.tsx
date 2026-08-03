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
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useDebounce } from '@/hooks/use-debounce'

import { searchUidOptions, upsertPlanAllocation } from '../api'
import type { PlanAllocation } from '../types'

const FORM_ID = 'plan-allocation-form'

type FormValues = {
  client_user_id: string
  allocated_quota: string
}

type AllocationFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  planId: number
  /** Present when editing; the client UID is then fixed */
  currentAllocation?: PlanAllocation
  onSaved: () => void
}

export function AllocationFormDialog({
  open,
  onOpenChange,
  planId,
  currentAllocation,
  onSaved,
}: AllocationFormDialogProps) {
  const { t } = useTranslation()
  const isEdit = !!currentAllocation
  const [uidKeyword, setUidKeyword] = useState('')
  const debouncedKeyword = useDebounce(uidKeyword, 300)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const uidOptionsQuery = useQuery({
    queryKey: ['project-budget', 'uid-options', debouncedKeyword],
    queryFn: () => searchUidOptions(debouncedKeyword),
    enabled: open && !isEdit,
  })

  const form = useForm<FormValues>({
    defaultValues: { client_user_id: '', allocated_quota: '0' },
  })

  useEffect(() => {
    if (!open) return
    form.reset({
      client_user_id: currentAllocation?.client_user_id ?? '',
      allocated_quota: String(currentAllocation?.allocated_quota ?? 0),
    })
  }, [currentAllocation, form, open])

  const handleSubmit = async (values: FormValues) => {
    setIsSubmitting(true)
    try {
      const res = await upsertPlanAllocation(planId, {
        client_user_id: values.client_user_id,
        allocated_quota: Number.parseInt(values.allocated_quota, 10) || 0,
      })
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
      title={isEdit ? t('Edit allocation') : t('Create allocation')}
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
          <Label htmlFor='allocation-uid'>{t('Client UID')} *</Label>
          {isEdit ? (
            <Input
              id='allocation-uid'
              disabled
              value={currentAllocation?.client_user_id ?? ''}
            />
          ) : (
            <Select
              items={uidOptionsQuery.data}
              value={form.watch('client_user_id')}
              onValueChange={(value) =>
                form.setValue('client_user_id', value ?? '', {
                  shouldValidate: true,
                })
              }
            >
              <SelectTrigger id='allocation-uid'>
                <SelectValue placeholder={t('Search UID')} />
              </SelectTrigger>
              <SelectContent>
                <div className='p-1'>
                  <Input
                    placeholder={t('Search UID')}
                    value={uidKeyword}
                    onChange={(event) => setUidKeyword(event.target.value)}
                  />
                </div>
                {(uidOptionsQuery.data ?? []).map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='allocation-quota'>{t('Allocated budget')}</Label>
          <Input
            id='allocation-quota'
            type='number'
            min={0}
            placeholder={t('Enter the allocated budget')}
            {...form.register('allocated_quota')}
          />
        </div>
      </form>
    </Dialog>
  )
}

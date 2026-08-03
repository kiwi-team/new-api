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
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { createProjectPlan, updateProjectPlan } from '../api'
import type { ProjectPlan } from '../types'

const FORM_ID = 'project-plan-form'

/** Plans store dates as `YYYYMMDD` digits. */
const PLAN_DATE_PATTERN = /^\d{8}$/

type FormValues = {
  plan_name: string
  start_date: string
  end_date: string
}

type PlanFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  projectId: number
  currentPlan?: ProjectPlan | null
  onSaved: () => void
}

export function PlanFormDialog({
  open,
  onOpenChange,
  projectId,
  currentPlan,
  onSaved,
}: PlanFormDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isEdit = !!currentPlan
  const [isSubmitting, setIsSubmitting] = useState(false)

  const form = useForm<FormValues>({
    defaultValues: { plan_name: '', start_date: '', end_date: '' },
  })

  useEffect(() => {
    if (!open) return
    form.reset({
      plan_name: currentPlan?.plan_name ?? '',
      start_date: currentPlan?.start_date ?? '',
      end_date: currentPlan?.end_date ?? '',
    })
  }, [currentPlan, form, open])

  const handleSubmit = async (values: FormValues) => {
    setIsSubmitting(true)
    try {
      const res = currentPlan
        ? await updateProjectPlan(currentPlan.id, values)
        : await createProjectPlan(projectId, values)
      if (!res.success) {
        toast.error(res.message || t('Operation failed'))
        return
      }
      toast.success(
        isEdit ? t('Updated successfully') : t('Created successfully')
      )
      await queryClient.invalidateQueries({
        queryKey: ['project-budget', 'plans', projectId],
      })
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
      title={isEdit ? t('Edit plan') : t('Create plan')}
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
          <Label htmlFor='plan-name'>{t('Plan name')} *</Label>
          <Input
            id='plan-name'
            placeholder={t('Enter the plan name')}
            {...form.register('plan_name', { required: true })}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='plan-start'>{t('Start date')} *</Label>
          <Input
            id='plan-start'
            placeholder='20260408'
            {...form.register('start_date', {
              required: true,
              pattern: PLAN_DATE_PATTERN,
            })}
          />
          <p className='text-muted-foreground text-xs'>
            {t('Format: YYYYMMDD, e.g. 20260408')}
          </p>
          {form.formState.errors.start_date && (
            <p className='text-destructive text-xs'>
              {t('Format: YYYYMMDD, e.g. 20260408')}
            </p>
          )}
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='plan-end'>{t('End date')} *</Label>
          <Input
            id='plan-end'
            placeholder='20260430'
            {...form.register('end_date', {
              required: true,
              pattern: PLAN_DATE_PATTERN,
            })}
          />
          <p className='text-muted-foreground text-xs'>
            {t('Format: YYYYMMDD, e.g. 20260408')}
          </p>
          {form.formState.errors.end_date && (
            <p className='text-destructive text-xs'>
              {t('Format: YYYYMMDD, e.g. 20260408')}
            </p>
          )}
        </div>
      </form>
    </Dialog>
  )
}

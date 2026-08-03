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

import { createProject, updateProject } from '../api'
import type { Project } from '../types'

const FORM_ID = 'project-form'

type FormValues = {
  project_name: string
  total_budget: string
}

type ProjectMutateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: Project | null
  onSaved: () => void
}

export function ProjectMutateDialog({
  open,
  onOpenChange,
  currentRow,
  onSaved,
}: ProjectMutateDialogProps) {
  const { t } = useTranslation()
  const isEdit = !!currentRow
  const [isSubmitting, setIsSubmitting] = useState(false)

  const form = useForm<FormValues>({
    defaultValues: { project_name: '', total_budget: '0' },
  })

  useEffect(() => {
    if (!open) return
    form.reset({
      project_name: currentRow?.project_name ?? '',
      total_budget: String(currentRow?.total_budget ?? 0),
    })
  }, [currentRow, form, open])

  const handleSubmit = async (values: FormValues) => {
    const payload = {
      project_name: values.project_name.trim(),
      total_budget: Number.parseInt(values.total_budget, 10) || 0,
    }
    setIsSubmitting(true)
    try {
      const res = currentRow
        ? await updateProject(currentRow.id, payload)
        : await createProject(payload)
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
      title={isEdit ? t('Edit project') : t('Create project')}
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
          <Label htmlFor='project-name'>{t('Project name')} *</Label>
          <Input
            id='project-name'
            // The name is the project's identity; renaming is not supported.
            disabled={isEdit}
            placeholder={t('Enter the project name')}
            {...form.register('project_name', { required: true })}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='project-budget'>{t('Total budget')}</Label>
          <Input
            id='project-budget'
            type='number'
            min={0}
            placeholder={t('Enter the total budget')}
            {...form.register('total_budget')}
          />
        </div>
      </form>
    </Dialog>
  )
}

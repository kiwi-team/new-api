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
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { getUserGroups, getUserModels } from '@/lib/api'

import { batchAppendApiKeyModels } from '../api'

type BatchAppendModelsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onDone: () => void
}

/**
 * Add models to every key in a group.
 *
 * Deliberately not driven by the row selection: the endpoint matches keys by
 * group name server-side, so the dialog asks for a group rather than acting on
 * whatever happens to be checked.
 */
export function BatchAppendModelsDialog({
  open,
  onOpenChange,
  onDone,
}: BatchAppendModelsDialogProps) {
  const { t } = useTranslation()
  const [group, setGroup] = useState('')
  const [models, setModels] = useState<string[]>([])
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (!open) {
      setGroup('')
      setModels([])
    }
  }, [open])

  const { data: modelsData } = useQuery({
    queryKey: ['user-models'],
    queryFn: getUserModels,
    enabled: open,
  })
  const { data: groupsData } = useQuery({
    queryKey: ['user-groups'],
    queryFn: getUserGroups,
    enabled: open,
  })

  const groupOptions = Object.keys(groupsData?.data || {})
  const modelOptions = (modelsData?.data || []).map((model) => ({
    label: model,
    value: model,
  }))

  const handleSubmit = async () => {
    if (!group || models.length === 0) {
      toast.warning(t('Select a group and at least one model'))
      return
    }
    setIsSubmitting(true)
    try {
      const res = await batchAppendApiKeyModels(group, models)
      if (!res.success) {
        toast.error(res.message || t('Operation failed'))
        return
      }
      toast.success(
        t('Added models to {{count}} keys', { count: res.data ?? 0 })
      )
      onOpenChange(false)
      onDone()
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Batch append models')}
      description={t('Models are added to every key in the selected group')}
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
          <Button onClick={() => void handleSubmit()} disabled={isSubmitting}>
            {t('Confirm')}
          </Button>
        </>
      }
    >
      <div className='space-y-4'>
        <div className='grid gap-1.5'>
          <Label htmlFor='batch-append-group'>{t('Group')}</Label>
          <Select
            value={group}
            onValueChange={(value) => setGroup(value ?? '')}
            items={groupOptions.map((name) => ({ value: name, label: name }))}
          >
            <SelectTrigger id='batch-append-group'>
              <SelectValue placeholder={t('Select a group')} />
            </SelectTrigger>
            <SelectContent>
              {groupOptions.map((name) => (
                <SelectItem key={name} value={name}>
                  {name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className='grid gap-1.5'>
          <Label htmlFor='batch-append-models'>{t('Models')}</Label>
          <MultiSelect
            id='batch-append-models'
            options={modelOptions}
            selected={models}
            onChange={setModels}
            placeholder={t('Select models')}
          />
        </div>
      </div>
    </Dialog>
  )
}

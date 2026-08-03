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
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { getUserGroups } from '@/lib/api'

import { batchSetApiKeyGroup } from '../api'

type BatchSetGroupDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Keys the group is applied to. */
  ids: number[]
  onDone: () => void
}

/** Assign one group to every selected key. */
export function BatchSetGroupDialog({
  open,
  onOpenChange,
  ids,
  onDone,
}: BatchSetGroupDialogProps) {
  const { t } = useTranslation()
  const [group, setGroup] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (!open) setGroup('')
  }, [open])

  const { data: groupsData } = useQuery({
    queryKey: ['user-groups'],
    queryFn: getUserGroups,
    enabled: open,
  })
  const groupOptions = Object.keys(groupsData?.data || {})

  const handleSubmit = async () => {
    if (!group) {
      toast.warning(t('Select a group'))
      return
    }
    setIsSubmitting(true)
    try {
      const res = await batchSetApiKeyGroup(ids, group)
      if (!res.success) {
        toast.error(res.message || t('Operation failed'))
        return
      }
      toast.success(t('Group set on {{count}} keys', { count: res.data ?? 0 }))
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
      title={t('Set group')}
      description={t('Applies to {{count}} selected keys', {
        count: ids.length,
      })}
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
          <Button onClick={() => void handleSubmit()} disabled={isSubmitting}>
            {t('Confirm')}
          </Button>
        </>
      }
    >
      <div className='grid gap-1.5'>
        <Label htmlFor='batch-set-group'>{t('Group')}</Label>
        <Select
          value={group}
          onValueChange={(value) => setGroup(value ?? '')}
          items={groupOptions.map((name) => ({ value: name, label: name }))}
        >
          <SelectTrigger id='batch-set-group'>
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
    </Dialog>
  )
}

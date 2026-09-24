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
import { JsonCodeEditor } from '@/components/json-code-editor'
import { TagInput } from '@/components/tag-input'
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
import { Switch } from '@/components/ui/switch'

import {
  createModelRouteConfig,
  getChannelNameList,
  updateModelRouteConfig,
} from '../api'
import { parseBodyMatchDraft } from '../lib/body-match'
import type {
  ModelRouteConfig,
  RouteApplyMode,
  RouteRandomType,
} from '../types'
import { ChannelGroupEditor } from './channel-group-editor'

const FORM_ID = 'model-route-config-form'
type BodyMatchMode = 'keyword' | 'json'

type FormValues = {
  name: string
  priority: string
  max_retry: string
  random_type: RouteRandomType
  apply_mode: RouteApplyMode
  enabled: boolean
}

type RouteConfigMutateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: ModelRouteConfig | null
  onSaved: () => void
}

export function RouteConfigMutateDialog({
  open,
  onOpenChange,
  currentRow,
  onSaved,
}: RouteConfigMutateDialogProps) {
  const { t } = useTranslation()
  const isEdit = !!currentRow
  const [isSubmitting, setIsSubmitting] = useState(false)
  // Pattern lists and channel groups are array-shaped, so they live beside
  // the form rather than inside it.
  const [modelPatterns, setModelPatterns] = useState<string[]>([])
  const [bodyPatterns, setBodyPatterns] = useState<string[]>([])
  const [bodyMatchDraft, setBodyMatchDraft] = useState('')
  const [bodyMatchMode, setBodyMatchMode] = useState<BodyMatchMode>('keyword')
  const [urlPatterns, setUrlPatterns] = useState<string[]>([])
  const [channelGroups, setChannelGroups] = useState<number[][]>([[]])

  const channelsQuery = useQuery({
    queryKey: ['model-route-config', 'channels'],
    queryFn: getChannelNameList,
    enabled: open,
  })

  const form = useForm<FormValues>({
    defaultValues: {
      name: '',
      priority: '0',
      max_retry: '3',
      random_type: 'order',
      apply_mode: 'fallback',
      enabled: true,
    },
  })

  useEffect(() => {
    if (!open) return
    form.reset({
      name: currentRow?.name ?? '',
      priority: String(currentRow?.priority ?? 0),
      max_retry: String(currentRow?.max_retry ?? 3),
      random_type: currentRow?.random_type ?? 'order',
      apply_mode: currentRow?.apply_mode ?? 'fallback',
      enabled: currentRow ? currentRow.enabled === 1 : true,
    })
    setModelPatterns(currentRow?.model_patterns ?? [])
    setBodyPatterns(currentRow?.body_patterns ?? [])
    setBodyMatchDraft(
      currentRow?.body_match
        ? JSON.stringify(currentRow.body_match, null, 2)
        : ''
    )
    setBodyMatchMode(currentRow?.body_match ? 'json' : 'keyword')
    setUrlPatterns(currentRow?.url_patterns ?? [])
    setChannelGroups(
      currentRow?.channel_groups?.length ? currentRow.channel_groups : [[]]
    )
  }, [currentRow, form, open])

  const handleSubmit = async (values: FormValues) => {
    if (modelPatterns.length === 0) {
      toast.warning(t('Configure at least one model name pattern'))
      return
    }
    if (channelGroups.some((group) => group.length === 0)) {
      toast.warning(t('Every channel group needs at least one channel'))
      return
    }

    const bodyMatchResult =
      bodyMatchMode === 'json'
        ? parseBodyMatchDraft(bodyMatchDraft)
        : { value: null, error: null }
    if (bodyMatchResult.error === 'invalid_json') {
      toast.warning(t('Invalid JSON'))
      return
    }
    if (bodyMatchResult.error === 'non_empty_object_required') {
      toast.warning(t('Body JSON match must be a non-empty object'))
      return
    }

    const payload = {
      name: values.name.trim(),
      model_patterns: modelPatterns,
      body_patterns: bodyMatchMode === 'keyword' ? bodyPatterns : [],
      body_match: bodyMatchResult.value,
      url_patterns: urlPatterns,
      channel_groups: channelGroups,
      random_type: values.random_type,
      max_retry: Number.parseInt(values.max_retry, 10) || 0,
      priority: Number.parseInt(values.priority, 10) || 0,
      apply_mode: values.apply_mode,
      enabled: values.enabled ? 1 : 0,
    }

    setIsSubmitting(true)
    try {
      const res = currentRow
        ? await updateModelRouteConfig({ ...payload, id: currentRow.id })
        : await createModelRouteConfig(payload)
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

  const randomTypeItems = [
    { value: 'order', label: t('In order') },
    { value: 'random', label: t('Random') },
  ]
  const bodyMatchModeItems = [
    { value: 'keyword', label: t('Legacy keyword match') },
    { value: 'json', label: t('JSON field match') },
  ]
  const applyModeItems = [
    { value: 'fallback', label: t('Compatible fallback') },
    { value: 'enforce', label: t('Strict routing') },
  ]

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={isEdit ? t('Edit route config') : t('Create route config')}
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
          <Label htmlFor='route-name'>{t('Config name')} *</Label>
          <Input
            id='route-name'
            {...form.register('name', { required: true })}
          />
        </div>

        <div className='grid grid-cols-2 gap-3'>
          <div className='grid gap-1.5'>
            <Label htmlFor='route-priority'>{t('Priority')}</Label>
            <Input
              id='route-priority'
              type='number'
              {...form.register('priority')}
            />
            <p className='text-muted-foreground text-xs'>
              {t('Higher numbers are evaluated first')}
            </p>
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='route-max-retry'>{t('Max retries')}</Label>
            <Input
              id='route-max-retry'
              type='number'
              min={0}
              {...form.register('max_retry')}
            />
          </div>
        </div>

        <div className='flex items-center justify-between rounded-lg border p-3'>
          <Label htmlFor='route-enabled'>{t('Enabled')}</Label>
          <Switch
            id='route-enabled'
            checked={form.watch('enabled')}
            onCheckedChange={(checked) => form.setValue('enabled', checked)}
          />
        </div>

        <div className='grid gap-1.5'>
          <Label>{t('Model name patterns')}</Label>
          <TagInput
            value={modelPatterns}
            onChange={setModelPatterns}
            placeholder={t('Regular expressions, press Enter to add')}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label>{t('Request body matching')}</Label>
          <Select
            items={bodyMatchModeItems}
            value={bodyMatchMode}
            onValueChange={(value) => {
              const nextMode = (value as BodyMatchMode) ?? 'keyword'
              setBodyMatchMode(nextMode)
              if (nextMode === 'json') {
                form.setValue('apply_mode', 'enforce')
              }
            }}
          >
            <SelectTrigger className='w-56'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {bodyMatchModeItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {bodyMatchMode === 'keyword' ? (
            <TagInput
              value={bodyPatterns}
              onChange={setBodyPatterns}
              placeholder={t('Keywords, press Enter to add')}
            />
          ) : (
            <>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'The request body must contain these JSON fields and values; other fields do not affect matching'
                )}
              </p>
              <JsonCodeEditor
                id='route-body-match'
                value={bodyMatchDraft}
                onChange={setBodyMatchDraft}
                placeholder='{"thinking":{"type":"adaptive"}}'
                ariaLabel={t('Body JSON match')}
                heightClassName='h-40 min-h-40 max-h-40'
              />
            </>
          )}
        </div>
        <div className='grid gap-1.5'>
          <Label>{t('URL keywords')}</Label>
          <TagInput
            value={urlPatterns}
            onChange={setUrlPatterns}
            placeholder={t('Keywords, press Enter to add')}
          />
        </div>

        <div className='grid gap-1.5'>
          <Label>{t('Application mode')}</Label>
          <Select
            items={applyModeItems}
            value={form.watch('apply_mode')}
            onValueChange={(value) =>
              form.setValue(
                'apply_mode',
                (value as RouteApplyMode) ?? 'fallback'
              )
            }
          >
            <SelectTrigger className='w-56'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {applyModeItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className='text-muted-foreground text-xs'>
            {form.watch('apply_mode') === 'enforce'
              ? t(
                  'A matched rule only uses its configured channels and fails if none are available'
                )
              : t(
                  'This rule is used only when higher-precedence routing produces no channels'
                )}
          </p>
        </div>

        <div className='grid gap-1.5'>
          <Label>{t('Selection mode')}</Label>
          <Select
            items={randomTypeItems}
            value={form.watch('random_type')}
            onValueChange={(value) =>
              form.setValue(
                'random_type',
                (value as RouteRandomType) ?? 'order'
              )
            }
          >
            <SelectTrigger className='w-48'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {randomTypeItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className='grid gap-1.5'>
          <Label>{t('Channel groups')} *</Label>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Groups are tried in order; the next group is used only after every channel in the current one fails'
            )}
          </p>
          <ChannelGroupEditor
            value={channelGroups}
            onChange={setChannelGroups}
            channels={channelsQuery.data ?? []}
          />
        </div>
      </form>
    </Dialog>
  )
}

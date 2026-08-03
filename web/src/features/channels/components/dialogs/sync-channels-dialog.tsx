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
import { ArrowRight, CheckCircle2, XCircle } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { getEnabledSyncEnvironments } from '@/features/sync-environments/api'

import {
  type ChannelSyncMapping,
  type EnvironmentPreview,
  type SyncResult,
  previewChannelSync,
  syncChannels,
} from '../../lib/channel-sync'

/** Sentinel for "create a new channel in the target environment". */
const CREATE_NEW = '0'

type SyncChannelsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Channels selected in the table */
  channelIds: number[]
}

type Step = 'select' | 'map' | 'result'

/**
 * Push the selected channels to other deployments.
 *
 * The flow is deliberately three steps: pick environments, review what each
 * environment matched (and override the target per channel), then sync — a
 * blind push would silently overwrite an unrelated channel that happens to
 * carry the same name.
 */
export function SyncChannelsDialog({
  open,
  onOpenChange,
  channelIds,
}: SyncChannelsDialogProps) {
  const { t } = useTranslation()
  const [step, setStep] = useState<Step>('select')
  const [selectedEnvironments, setSelectedEnvironments] = useState<number[]>([])
  const [previews, setPreviews] = useState<EnvironmentPreview[]>([])
  const [mapping, setMapping] = useState<ChannelSyncMapping>({})
  const [results, setResults] = useState<SyncResult[]>([])
  const [isBusy, setIsBusy] = useState(false)

  const environmentsQuery = useQuery({
    queryKey: ['channels', 'sync-environments'],
    queryFn: getEnabledSyncEnvironments,
    enabled: open,
  })

  useEffect(() => {
    if (open) return
    setStep('select')
    setSelectedEnvironments([])
    setPreviews([])
    setMapping({})
    setResults([])
  }, [open])

  const handlePreview = async () => {
    if (selectedEnvironments.length === 0) {
      toast.warning(t('Please select at least one environment'))
      return
    }
    setIsBusy(true)
    try {
      const res = await previewChannelSync({
        channel_ids: channelIds,
        environment_ids: selectedEnvironments,
      })
      if (!res.success) {
        toast.error(res.message || t('Operation failed'))
        return
      }
      const items = res.data ?? []
      setPreviews(items)
      // Default each channel to its first match, or to creating a new one.
      const nextMapping: ChannelSyncMapping = {}
      for (const preview of items) {
        nextMapping[preview.environment_id] = {}
        for (const channel of preview.channels) {
          nextMapping[preview.environment_id][channel.channel_id] =
            channel.matches[0]?.id ?? 0
        }
      }
      setMapping(nextMapping)
      setStep('map')
    } finally {
      setIsBusy(false)
    }
  }

  const handleSync = async () => {
    setIsBusy(true)
    try {
      const res = await syncChannels({
        channel_ids: channelIds,
        environment_ids: selectedEnvironments,
        channel_mapping: mapping,
      })
      if (!res.success) {
        toast.error(res.message || t('Operation failed'))
        return
      }
      setResults(res.data ?? [])
      setStep('result')
    } finally {
      setIsBusy(false)
    }
  }

  const environments = environmentsQuery.data ?? []

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Sync channels to other environments')}
      description={t('{{count}} channels selected', {
        count: channelIds.length,
      })}
      contentClassName='sm:max-w-3xl'
      contentHeight='auto'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {step === 'result' ? t('Close') : t('Cancel')}
          </Button>
          {step === 'select' && (
            <Button onClick={() => void handlePreview()} disabled={isBusy}>
              {t('Preview')}
            </Button>
          )}
          {step === 'map' && (
            <Button onClick={() => void handleSync()} disabled={isBusy}>
              {t('Start sync')}
            </Button>
          )}
        </>
      }
    >
      {step === 'select' && (
        <div className='space-y-3'>
          {environments.length === 0 && (
            <p className='text-muted-foreground text-sm'>
              {t('No enabled environments. Add one under Environments first.')}
            </p>
          )}
          {environments.map((environment) => (
            <label
              key={environment.id}
              className='hover:bg-muted/50 flex items-start gap-3 rounded-lg border p-3'
            >
              <Checkbox
                checked={selectedEnvironments.includes(environment.id)}
                onCheckedChange={(checked) =>
                  setSelectedEnvironments((prev) =>
                    checked
                      ? [...prev, environment.id]
                      : prev.filter((id) => id !== environment.id)
                  )
                }
              />
              <div className='space-y-0.5'>
                <div className='text-sm font-medium'>{environment.name}</div>
                <div className='text-muted-foreground text-xs'>
                  {environment.api_url}
                </div>
                {environment.remark && (
                  <div className='text-muted-foreground text-xs'>
                    {environment.remark}
                  </div>
                )}
              </div>
            </label>
          ))}
        </div>
      )}

      {step === 'map' && (
        <div className='space-y-4'>
          {previews.map((preview) => (
            <div key={preview.environment_id} className='space-y-2'>
              <div className='flex items-center gap-2'>
                <span className='text-sm font-medium'>
                  {preview.environment_name}
                </span>
                {preview.error && (
                  <Badge variant='destructive'>{preview.error}</Badge>
                )}
              </div>
              {preview.channels.map((channel) => {
                const items = [
                  { value: CREATE_NEW, label: t('Create a new channel') },
                  ...channel.matches.map((match) => ({
                    value: String(match.id),
                    label: `#${match.id} ${match.name}`,
                  })),
                ]
                return (
                  <div
                    key={channel.channel_id}
                    className='flex items-center gap-3 rounded-lg border p-3'
                  >
                    <div className='min-w-0 flex-1'>
                      <div className='truncate text-sm'>
                        {channel.channel_name}
                      </div>
                      <div className='text-muted-foreground truncate text-xs'>
                        {channel.models}
                      </div>
                    </div>
                    <ArrowRight className='text-muted-foreground h-4 w-4 shrink-0' />
                    <div className='w-64 shrink-0'>
                      <Label className='sr-only'>{t('Target channel')}</Label>
                      <Select
                        items={items}
                        value={String(
                          mapping[preview.environment_id]?.[
                            channel.channel_id
                          ] ?? 0
                        )}
                        onValueChange={(value) =>
                          setMapping((prev) => ({
                            ...prev,
                            [preview.environment_id]: {
                              ...prev[preview.environment_id],
                              [channel.channel_id]: Number.parseInt(
                                value ?? CREATE_NEW,
                                10
                              ),
                            },
                          }))
                        }
                      >
                        <SelectTrigger className='w-full'>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {items.map((item) => (
                            <SelectItem key={item.value} value={item.value}>
                              {item.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                )
              })}
            </div>
          ))}
        </div>
      )}

      {step === 'result' && (
        <div className='space-y-3'>
          {results.map((result) => (
            <div key={result.environment_id} className='rounded-lg border p-3'>
              <div className='flex items-center gap-2'>
                {result.success ? (
                  <CheckCircle2 className='h-4 w-4 text-emerald-600 dark:text-emerald-400' />
                ) : (
                  <XCircle className='text-destructive h-4 w-4' />
                )}
                <span className='text-sm font-medium'>
                  {result.environment_name}
                </span>
                {result.success ? (
                  <span className='text-muted-foreground text-xs'>
                    {t('{{count}} channels synced', {
                      count: result.synced_count ?? 0,
                    })}
                  </span>
                ) : (
                  <span className='text-destructive text-xs'>
                    {result.error}
                  </span>
                )}
              </div>
              {result.details && result.details.length > 0 && (
                <div className='mt-2 space-y-1 ps-6'>
                  {result.details.map((detail) => (
                    <div
                      key={detail.channel_id}
                      className='text-muted-foreground flex items-center gap-2 text-xs'
                    >
                      <Badge variant='secondary'>{detail.action}</Badge>
                      <span>{detail.channel_name}</span>
                      {!detail.success && (
                        <span className='text-destructive'>{detail.error}</span>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </Dialog>
  )
}

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
import { CircleCheck, CircleX, Search } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { getEnabledSyncEnvironments } from '@/features/sync-environments/api'

import {
  type ModelPriceSyncResult,
  collectPricedModels,
  syncModelPrices,
} from './model-price-sync'

type SyncMode = 'all' | 'selected'

type SyncModelPricesDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Raw option blobs; the picker is derived from them. */
  options: {
    ModelPrice?: string
    ModelRatio?: string
    CompletionRatio?: string
  }
}

export function SyncModelPricesDialog({
  open,
  onOpenChange,
  options,
}: SyncModelPricesDialogProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<SyncMode>('all')
  const [selectedModels, setSelectedModels] = useState<string[]>([])
  const [selectedEnvIds, setSelectedEnvIds] = useState<number[]>([])
  const [keyword, setKeyword] = useState('')
  const [isSyncing, setIsSyncing] = useState(false)
  const [results, setResults] = useState<ModelPriceSyncResult[] | null>(null)

  useEffect(() => {
    if (open) return
    setMode('all')
    setSelectedModels([])
    setSelectedEnvIds([])
    setKeyword('')
    setResults(null)
  }, [open])

  const { data: environments } = useQuery({
    queryKey: ['sync-environments', 'enabled'],
    queryFn: getEnabledSyncEnvironments,
    enabled: open,
  })

  const allModels = useMemo(() => collectPricedModels(options), [options])
  const visibleModels = useMemo(() => {
    const needle = keyword.trim().toLowerCase()
    if (!needle) return allModels
    return allModels.filter((name) => name.toLowerCase().includes(needle))
  }, [allModels, keyword])

  const handleSync = async () => {
    if (selectedEnvIds.length === 0) {
      toast.warning(t('Select at least one target environment'))
      return
    }
    if (mode === 'selected' && selectedModels.length === 0) {
      toast.warning(t('Select at least one model'))
      return
    }

    setIsSyncing(true)
    try {
      const res = await syncModelPrices({
        environment_ids: selectedEnvIds,
        // An empty list is the backend's signal for a full replace, so the
        // selection is only sent in "selected" mode.
        ...(mode === 'selected' ? { selected_models: selectedModels } : {}),
      })
      if (!res.success) {
        toast.error(res.message || t('Operation failed'))
        return
      }
      const list = res.results ?? []
      setResults(list)
      const okCount = list.filter((item) => item.success).length
      if (okCount === list.length) {
        toast.success(t('Synced to {{count}} environments', { count: okCount }))
      } else {
        toast.warning(
          t('{{ok}} of {{total}} environments succeeded', {
            ok: okCount,
            total: list.length,
          })
        )
      }
    } finally {
      setIsSyncing(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Sync model prices')}
      description={t(
        'Push the local model pricing to other deployments of this application.'
      )}
      contentClassName='sm:max-w-2xl'
      contentHeight='min(75dvh, 720px)'
      footer={
        results ? (
          <Button onClick={() => onOpenChange(false)}>{t('Close')}</Button>
        ) : (
          <>
            <Button variant='outline' onClick={() => onOpenChange(false)}>
              {t('Cancel')}
            </Button>
            <Button onClick={() => void handleSync()} disabled={isSyncing}>
              {t('Start sync')}
            </Button>
          </>
        )
      }
    >
      {results ? (
        <div className='space-y-2'>
          {results.map((result) => (
            <div
              key={result.environment_id}
              className='flex items-start gap-2 rounded-md border p-3 text-sm'
            >
              {result.success ? (
                <CircleCheck className='mt-0.5 h-4 w-4 shrink-0 text-emerald-500' />
              ) : (
                <CircleX className='text-destructive mt-0.5 h-4 w-4 shrink-0' />
              )}
              <div className='min-w-0'>
                <div className='font-medium'>{result.environment_name}</div>
                {result.success ? (
                  <div className='text-muted-foreground text-xs'>
                    {t('{{count}} models synced', {
                      count: result.synced_count ?? 0,
                    })}
                  </div>
                ) : (
                  <div className='text-destructive text-xs break-all'>
                    {result.error}
                  </div>
                )}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className='space-y-5'>
          <div className='space-y-2'>
            <Label>{t('What to sync')}</Label>
            <RadioGroup
              value={mode}
              onValueChange={(value) => setMode(value as SyncMode)}
            >
              <label className='flex items-start gap-2 rounded-md border p-3'>
                <RadioGroupItem value='all' className='mt-0.5' />
                <div>
                  <div className='text-sm font-medium'>{t('All models')}</div>
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      'Replaces the pricing on each target with the local one.'
                    )}
                  </p>
                </div>
              </label>
              <label className='flex items-start gap-2 rounded-md border p-3'>
                <RadioGroupItem value='selected' className='mt-0.5' />
                <div>
                  <div className='text-sm font-medium'>
                    {t('Selected models')}
                  </div>
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      'Merges the selected models into the pricing each target already has.'
                    )}
                  </p>
                </div>
              </label>
            </RadioGroup>
          </div>

          {mode === 'selected' && (
            <div className='space-y-2'>
              <div className='flex items-center justify-between gap-2'>
                <Label>
                  {t('Models')} ({selectedModels.length}/{allModels.length})
                </Label>
                <div className='relative'>
                  <Search className='text-muted-foreground pointer-events-none absolute inset-y-0 start-2 my-auto h-4 w-4' />
                  <Input
                    className='w-48 ps-8'
                    placeholder={t('Filter models')}
                    value={keyword}
                    onChange={(event) => setKeyword(event.target.value)}
                  />
                </div>
              </div>
              <div className='max-h-64 space-y-1 overflow-auto rounded-md border p-2'>
                {visibleModels.length === 0 && (
                  <p className='text-muted-foreground py-4 text-center text-sm'>
                    {t('No priced models match')}
                  </p>
                )}
                {visibleModels.map((name) => (
                  <label
                    key={name}
                    className='hover:bg-muted/50 flex items-center gap-2 rounded px-2 py-1 text-sm'
                  >
                    <Checkbox
                      checked={selectedModels.includes(name)}
                      onCheckedChange={(checked) =>
                        setSelectedModels((prev) =>
                          checked
                            ? [...prev, name]
                            : prev.filter((item) => item !== name)
                        )
                      }
                    />
                    <span className='font-mono text-xs'>{name}</span>
                  </label>
                ))}
              </div>
            </div>
          )}

          <div className='space-y-2'>
            <Label>{t('Target environments')}</Label>
            <div className='space-y-1 rounded-md border p-2'>
              {(environments ?? []).length === 0 && (
                <p className='text-muted-foreground py-4 text-center text-sm'>
                  {t('No enabled environments configured')}
                </p>
              )}
              {(environments ?? []).map((environment) => (
                <label
                  key={environment.id}
                  className='hover:bg-muted/50 flex items-center gap-2 rounded px-2 py-1 text-sm'
                >
                  <Checkbox
                    checked={selectedEnvIds.includes(environment.id)}
                    onCheckedChange={(checked) =>
                      setSelectedEnvIds((prev) =>
                        checked
                          ? [...prev, environment.id]
                          : prev.filter((id) => id !== environment.id)
                      )
                    }
                  />
                  <span>{environment.name}</span>
                  <span className='text-muted-foreground text-xs'>
                    {environment.api_url}
                  </span>
                </label>
              ))}
            </div>
          </div>
        </div>
      )}
    </Dialog>
  )
}

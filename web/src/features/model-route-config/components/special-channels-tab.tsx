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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { GripVertical, Plus, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { MultiSelect } from '@/components/multi-select'
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
import { Textarea } from '@/components/ui/textarea'
import {
  getSystemOptions,
  updateSystemOption,
} from '@/features/system-settings/api'

import { getChannelNameList } from '../api'
import {
  parseGeminiFileRouting,
  geminiFileRoutingSchema,
  type GeminiFileRouting,
  type SpecialChannelRule,
  createEmptySpecialChannelRule,
  createEmptySpecialChannelTier,
  parseSpecialChannelRules,
  serializeSpecialChannelRules,
} from '../lib/special-channels'

export function SpecialChannelsTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [ruleDraft, setRuleDraft] = useState<SpecialChannelRule[] | null>(null)
  const [configDraft, setConfigDraft] = useState<GeminiFileRouting | null>(null)
  const [drag, setDrag] = useState<{ rule: number; tier: number } | null>(null)

  const optionsQuery = useQuery({
    queryKey: ['system-options'],
    queryFn: getSystemOptions,
  })
  const { data: channelList } = useQuery({
    queryKey: ['channel-name-list'],
    queryFn: getChannelNameList,
    staleTime: 5 * 60 * 1000,
  })

  const optionValue = optionsQuery.data?.data?.find(
    (option) => option.key === 'LargeFileChannels'
  )?.value
  const configValue = optionsQuery.data?.data?.find(
    (option) => option.key === 'GeminiFileRouting'
  )?.value
  const parsed = useMemo(() => {
    try {
      return {
        rules: parseSpecialChannelRules(optionValue),
        config: parseGeminiFileRouting(configValue),
        error: false,
      }
    } catch {
      return { rules: [], config: parseGeminiFileRouting(), error: true }
    }
  }, [optionValue, configValue])
  const rules = ruleDraft ?? parsed.rules
  const config = configDraft ?? parsed.config
  const setRules = (
    update: (current: SpecialChannelRule[]) => SpecialChannelRule[]
  ) => setRuleDraft((current) => update(current ?? parsed.rules))
  const patchConfig = (patch: Partial<GeminiFileRouting>) =>
    setConfigDraft((current) => ({ ...(current ?? parsed.config), ...patch }))

  const channelOptions = (channelList ?? []).map((channel) => ({
    label: `${channel.name} (${channel.id})`,
    value: String(channel.id),
  }))

  const saveMutation = useMutation({
    mutationFn: async () => {
      const nextConfig = geminiFileRoutingSchema.parse({
        ...config,
        chat_model_patterns: config.chat_model_patterns
          .map((pattern) => pattern.trim())
          .filter(Boolean),
      })
      const nextRules = serializeSpecialChannelRules(rules)
      const result = await updateSystemOption({
        key: 'LargeFileChannels',
        value: nextRules,
      })
      if (!result.success) return result
      return updateSystemOption({
        key: 'GeminiFileRouting',
        value: JSON.stringify(nextConfig),
      })
    },
    onError: () =>
      toast.error(t('Check the file routing settings and try again.')),
    onSuccess: async (res) => {
      if (!res.success) {
        toast.error(res.message || t('Operation failed'))
        return
      }
      toast.success(t('Saved successfully'))
      await queryClient.invalidateQueries({ queryKey: ['system-options'] })
      setRuleDraft(null)
      setConfigDraft(null)
    },
  })

  const patchRule = (index: number, patch: Partial<SpecialChannelRule>) =>
    setRules((current) =>
      current.map((rule, i) => (i === index ? { ...rule, ...patch } : rule))
    )

  const handleDrop = (ruleIndex: number, tierIndex: number) => {
    if (!drag || drag.rule !== ruleIndex) return
    const tiers = [...rules[ruleIndex].tiers]
    const [moved] = tiers.splice(drag.tier, 1)
    tiers.splice(tierIndex, 0, moved)
    patchRule(ruleIndex, { tiers })
    setDrag(null)
  }

  if (parsed.error || optionsQuery.isError) {
    return (
      <ErrorState
        description={t(
          'File routing configuration is invalid. Correct the saved configuration before editing.'
        )}
        onRetry={() => void optionsQuery.refetch()}
      />
    )
  }

  return (
    <div className='flex h-full min-h-0 flex-col gap-4'>
      <div className='grid gap-3 rounded-lg border p-3'>
        <div className='flex items-center justify-between'>
          <Label htmlFor='gemini-file-enabled'>{t('Large file routing')}</Label>
          <Switch
            id='gemini-file-enabled'
            checked={config.enabled}
            onCheckedChange={(enabled) => patchConfig({ enabled })}
          />
        </div>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Only URL attachments use the file size threshold. Base64 stays inline and does not trigger large file routing.'
          )}
        </p>
        <div className='grid gap-3 sm:grid-cols-3'>
          <div className='grid gap-1.5'>
            <Label htmlFor='gemini-file-mode'>
              {t('File processing mode')}
            </Label>
            <Select
              value={config.mode}
              onValueChange={(mode) =>
                patchConfig({
                  mode: mode === 'process' ? 'process' : 'forward',
                })
              }
              items={[
                { value: 'forward', label: t('Forward to another site') },
                { value: 'process', label: t('Upload to GCS locally') },
              ]}
            >
              <SelectTrigger id='gemini-file-mode'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='forward'>
                  {t('Forward to another site')}
                </SelectItem>
                <SelectItem value='process'>
                  {t('Upload to GCS locally')}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='gemini-file-threshold'>
              {t('URL file threshold (bytes)')}
            </Label>
            <Input
              id='gemini-file-threshold'
              type='number'
              min={1}
              max={16777216}
              value={config.inline_threshold_bytes}
              onChange={(event) =>
                patchConfig({
                  inline_threshold_bytes: Number(event.target.value),
                })
              }
            />
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='gemini-file-request-limit'>
              {t('Upstream request limit (bytes)')}
            </Label>
            <Input
              id='gemini-file-request-limit'
              type='number'
              min={1}
              value={config.max_inline_request_bytes}
              onChange={(event) =>
                patchConfig({
                  max_inline_request_bytes: Number(event.target.value),
                })
              }
            />
          </div>
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='gemini-file-models'>
            {t('Chat model patterns (one per line)')}
          </Label>
          <Textarea
            id='gemini-file-models'
            value={config.chat_model_patterns.join('\n')}
            placeholder='^gemini-'
            onChange={(event) =>
              patchConfig({
                chat_model_patterns: event.target.value.split('\n'),
              })
            }
          />
        </div>
      </div>

      <div className='flex items-center justify-between gap-2'>
        <p className='text-muted-foreground text-sm'>
          {t('Large file channels')}
        </p>
        <div className='flex gap-2'>
          <Button
            variant='outline'
            size='sm'
            onClick={() =>
              setRules((current) => [
                ...current,
                createEmptySpecialChannelRule(),
              ])
            }
          >
            <Plus className='h-4 w-4' />
            {t('Add rule')}
          </Button>
          <Button
            size='sm'
            disabled={
              saveMutation.isPending ||
              optionsQuery.isPending ||
              parsed.error ||
              optionsQuery.isError
            }
            onClick={() => saveMutation.mutate()}
          >
            {t('Save')}
          </Button>
        </div>
      </div>

      <p className='text-muted-foreground text-xs'>
        {t(
          'Large file requests use these channels, including retries. Models match exactly first, then by regular expression.'
        )}
      </p>

      <div className='min-h-0 flex-1 space-y-3 overflow-auto'>
        {rules.length === 0 && (
          <EmptyState
            title={t('No large file rules')}
            description={t(
              'Large file requests fail when routing is enabled without a matching rule.'
            )}
          />
        )}

        {rules.map((rule, ruleIndex) => (
          <div key={rule.uid} className='space-y-3 rounded-lg border p-3'>
            <div className='grid grid-cols-1 gap-3 sm:grid-cols-[1fr_auto_auto]'>
              <div className='grid gap-1.5'>
                <Label htmlFor={`sc-model-${ruleIndex}`}>
                  {t('Model name or prefix')}
                </Label>
                <Input
                  id={`sc-model-${ruleIndex}`}
                  value={rule.modelName}
                  placeholder='^gemini-'
                  onChange={(event) =>
                    patchRule(ruleIndex, { modelName: event.target.value })
                  }
                />
              </div>
              <div className='grid gap-1.5'>
                <Label htmlFor={`sc-random-${ruleIndex}`}>
                  {t('Selection')}
                </Label>
                <Select
                  value={rule.randomType}
                  onValueChange={(next) =>
                    patchRule(ruleIndex, {
                      randomType: next === 'random' ? 'random' : 'order',
                    })
                  }
                  items={[
                    { value: 'order', label: t('In order') },
                    { value: 'random', label: t('Random') },
                  ]}
                >
                  <SelectTrigger id={`sc-random-${ruleIndex}`} className='w-32'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='order'>{t('In order')}</SelectItem>
                    <SelectItem value='random'>{t('Random')}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className='flex items-end'>
                <Button
                  variant='ghost'
                  size='icon'
                  className='text-destructive'
                  aria-label={t('Delete rule')}
                  onClick={() =>
                    setRules((current) =>
                      current.filter((_, i) => i !== ruleIndex)
                    )
                  }
                >
                  <Trash2 className='h-4 w-4' />
                </Button>
              </div>
            </div>

            <div className='space-y-2'>
              <div className='flex items-center justify-between'>
                <Label>{t('Channel tiers')}</Label>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() =>
                    patchRule(ruleIndex, {
                      tiers: [...rule.tiers, createEmptySpecialChannelTier()],
                    })
                  }
                >
                  <Plus className='h-4 w-4' />
                  {t('Add tier')}
                </Button>
              </div>

              {rule.tiers.map((tier, tierIndex) => (
                <div
                  key={tier.uid}
                  className='bg-muted/30 flex items-center gap-2 rounded-md p-2'
                  draggable
                  onDragStart={() =>
                    setDrag({ rule: ruleIndex, tier: tierIndex })
                  }
                  onDragOver={(event) => event.preventDefault()}
                  onDrop={() => handleDrop(ruleIndex, tierIndex)}
                  onDragEnd={() => setDrag(null)}
                >
                  <GripVertical className='text-muted-foreground h-4 w-4 shrink-0 cursor-move' />
                  <span className='text-muted-foreground w-6 shrink-0 text-xs'>
                    #{tierIndex + 1}
                  </span>
                  <div className='min-w-0 flex-1'>
                    <MultiSelect
                      options={channelOptions}
                      selected={tier.ids.map(String)}
                      onChange={(values) =>
                        patchRule(ruleIndex, {
                          tiers: rule.tiers.map((item, i) =>
                            i === tierIndex
                              ? {
                                  ...item,
                                  ids: values.map(Number).filter(Boolean),
                                }
                              : item
                          ),
                        })
                      }
                      placeholder={t('Select channels')}
                    />
                  </div>
                  <Button
                    variant='ghost'
                    size='icon'
                    className='text-destructive shrink-0'
                    aria-label={t('Delete tier')}
                    onClick={() =>
                      patchRule(ruleIndex, {
                        tiers: rule.tiers.filter((_, i) => i !== tierIndex),
                      })
                    }
                  >
                    <Trash2 className='h-4 w-4' />
                  </Button>
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

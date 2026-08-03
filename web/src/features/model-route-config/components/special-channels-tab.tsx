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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  getSystemOptions,
  updateSystemOption,
} from '@/features/system-settings/api'

import { getChannelNameList } from '../api'
import {
  SPECIAL_CHANNEL_KEYS,
  type SpecialChannelKey,
  type SpecialChannelRule,
  createEmptySpecialChannelRule,
  createEmptySpecialChannelTier,
  parseSpecialChannelRules,
  serializeSpecialChannelRules,
} from '../lib/special-channels'

/** Explains which requests each option key applies to. */
const KEY_DESCRIPTIONS: Record<SpecialChannelKey, string> = {
  OnlyTextChannels: 'Text-only requests (no modality tag)',
  VideoChannels: 'Video requests (video tag)',
  OnlyImageChannels: 'Image-only requests (image tag)',
  NoVideoChannels: 'Multimodal requests without video',
}

export function SpecialChannelsTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [activeKey, setActiveKey] = useState<SpecialChannelKey>(
    SPECIAL_CHANNEL_KEYS[0]
  )
  const [rules, setRules] = useState<SpecialChannelRule[]>([])
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
    (option) => option.key === activeKey
  )?.value

  useEffect(() => {
    setRules(parseSpecialChannelRules(optionValue))
  }, [optionValue, activeKey])

  const channelOptions = (channelList ?? []).map((channel) => ({
    label: `${channel.name} (${channel.id})`,
    value: String(channel.id),
  }))

  const saveMutation = useMutation({
    mutationFn: (next: SpecialChannelRule[]) =>
      updateSystemOption({
        key: activeKey,
        value: serializeSpecialChannelRules(next),
      }),
    onSuccess: async (res) => {
      if (!res.success) {
        toast.error(res.message || t('Operation failed'))
        return
      }
      toast.success(t('Saved successfully'))
      await queryClient.invalidateQueries({ queryKey: ['system-options'] })
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

  return (
    <div className='flex h-full min-h-0 flex-col gap-4'>
      <Tabs
        value={activeKey}
        onValueChange={(key) => setActiveKey(key as SpecialChannelKey)}
      >
        <TabsList>
          {SPECIAL_CHANNEL_KEYS.map((key) => (
            <TabsTrigger key={key} value={key}>
              {key}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      <div className='flex items-center justify-between gap-2'>
        <p className='text-muted-foreground text-sm'>
          {t(KEY_DESCRIPTIONS[activeKey])}
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
            disabled={saveMutation.isPending}
            onClick={() => saveMutation.mutate(rules)}
          >
            {t('Save')}
          </Button>
        </div>
      </div>

      <p className='text-muted-foreground text-xs'>
        {t(
          'These rules override a key’s own channel rules. Model name is matched exactly first, then as a regular expression.'
        )}
      </p>

      <div className='min-h-0 flex-1 space-y-3 overflow-auto'>
        {rules.length === 0 && (
          <p className='text-muted-foreground py-8 text-center text-sm'>
            {t('No rules yet. All models use the default routing.')}
          </p>
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
                  <SelectTrigger
                    id={`sc-random-${ruleIndex}`}
                    className='w-32'
                  >
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

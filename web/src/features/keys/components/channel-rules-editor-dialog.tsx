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
import { GripVertical, Plus, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
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
import { getChannelNameList } from '@/features/channels/api'
import { getUserModels } from '@/lib/api'

import {
  type ChannelRule,
  RANDOM_TYPES,
  createEmptyRule,
  createEmptyTier,
  parseChannelRules,
  reorderTiers,
  serializeChannelRules,
} from '../lib/channel-rules'

type ChannelRulesEditorDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Current JSON value from the form. */
  value: string
  onSave: (json: string) => void
}

/**
 * Visual editor for a key's channel routing rules.
 *
 * One card per model rule; inside it, an ordered list of channel tiers the
 * relay tries in turn. Tiers are drag-reorderable because their order is the
 * fallback priority. Fields the backend supports but this editor does not
 * surface (`group_ratio`, `name`, `weight`) survive editing untouched.
 */
export function ChannelRulesEditorDialog({
  open,
  onOpenChange,
  value,
  onSave,
}: ChannelRulesEditorDialogProps) {
  const { t } = useTranslation()
  const [rules, setRules] = useState<ChannelRule[]>([])
  const [drag, setDrag] = useState<{ rule: number; tier: number } | null>(null)

  useEffect(() => {
    if (!open) return
    const parsed = parseChannelRules(value)
    // An unparseable value would silently become "no rules" on save, so keep
    // the raw JSON and let the user fix it in the textarea instead.
    if (value.trim() && parsed.length === 0) {
      toast.warning(t('Existing rules could not be parsed; edit the JSON directly'))
      onOpenChange(false)
      return
    }
    setRules(parsed)
  }, [open, value, onOpenChange, t])

  const { data: channelList } = useQuery({
    queryKey: ['channel-name-list'],
    queryFn: getChannelNameList,
    enabled: open,
    staleTime: 5 * 60 * 1000,
  })
  const { data: modelsData } = useQuery({
    queryKey: ['user-models'],
    queryFn: getUserModels,
    enabled: open,
  })

  const channelOptions = (channelList ?? []).map((channel) => ({
    label: `${channel.name} (${channel.id})`,
    value: String(channel.id),
  }))
  const modelOptions = (modelsData?.data ?? []).map((model) => ({
    label: model,
    value: model,
  }))

  const patchRule = (index: number, patch: Partial<ChannelRule>) =>
    setRules((current) =>
      current.map((rule, i) => (i === index ? { ...rule, ...patch } : rule))
    )

  const handleDrop = (ruleIndex: number, tierIndex: number) => {
    if (!drag || drag.rule !== ruleIndex) return
    patchRule(ruleIndex, {
      tiers: reorderTiers(rules[ruleIndex].tiers, drag.tier, tierIndex),
    })
    setDrag(null)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Channel rules')}
      description={t(
        'Per-model routing: tiers are tried in order, so the first tier is the preferred one'
      )}
      contentClassName='sm:max-w-3xl'
      contentHeight='min(75dvh, 760px)'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={() => {
              onSave(serializeChannelRules(rules))
              onOpenChange(false)
            }}
          >
            {t('Save')}
          </Button>
        </>
      }
    >
      <div className='space-y-3'>
        <Button
          variant='outline'
          size='sm'
          onClick={() => setRules((current) => [...current, createEmptyRule()])}
        >
          <Plus className='h-4 w-4' />
          {t('Add rule')}
        </Button>

        {rules.length === 0 && (
          <p className='text-muted-foreground py-8 text-center text-sm'>
            {t('No rules yet. All models use the default routing.')}
          </p>
        )}

        {rules.map((rule, ruleIndex) => (
          <div
            key={rule.uid}
            className='space-y-3 rounded-lg border p-3'
          >
            <div className='grid grid-cols-1 gap-3 sm:grid-cols-[1fr_auto_auto_auto]'>
              <div className='grid gap-1.5'>
                <Label htmlFor={`rule-model-${ruleIndex}`}>
                  {t('Model name or prefix')}
                </Label>
                <Input
                  id={`rule-model-${ruleIndex}`}
                  list={`rule-models-${ruleIndex}`}
                  value={rule.modelKey}
                  placeholder='gpt-4o'
                  onChange={(event) =>
                    patchRule(ruleIndex, { modelKey: event.target.value })
                  }
                />
                <datalist id={`rule-models-${ruleIndex}`}>
                  {modelOptions.map((option) => (
                    <option key={option.value} value={option.value} />
                  ))}
                </datalist>
              </div>

              <div className='grid gap-1.5'>
                <Label htmlFor={`rule-retry-${ruleIndex}`}>{t('Retry')}</Label>
                <Input
                  id={`rule-retry-${ruleIndex}`}
                  className='w-24'
                  inputMode='numeric'
                  value={String(rule.retry)}
                  onChange={(event) =>
                    patchRule(ruleIndex, {
                      retry: Math.max(
                        0,
                        Number.parseInt(event.target.value, 10) || 0
                      ),
                    })
                  }
                />
              </div>

              <div className='grid gap-1.5'>
                <Label htmlFor={`rule-random-${ruleIndex}`}>
                  {t('Selection')}
                </Label>
                <Select
                  value={rule.randomType}
                  onValueChange={(next) =>
                    patchRule(ruleIndex, {
                      randomType: next === 'random' ? 'random' : 'order',
                    })
                  }
                  items={RANDOM_TYPES.map((type) => ({
                    value: type,
                    label: type === 'order' ? t('In order') : t('Random'),
                  }))}
                >
                  <SelectTrigger
                    id={`rule-random-${ruleIndex}`}
                    className='w-32'
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {RANDOM_TYPES.map((type) => (
                      <SelectItem key={type} value={type}>
                        {type === 'order' ? t('In order') : t('Random')}
                      </SelectItem>
                    ))}
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

            <div className='grid gap-1.5'>
              <Label>{t('Disabled channels')}</Label>
              <MultiSelect
                options={channelOptions}
                selected={rule.disableChannels.map(String)}
                onChange={(values) =>
                  patchRule(ruleIndex, {
                    disableChannels: values.map(Number).filter(Boolean),
                  })
                }
                placeholder={t('None')}
              />
            </div>

            <div className='space-y-2'>
              <div className='flex items-center justify-between'>
                <Label>{t('Channel tiers')}</Label>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() =>
                    patchRule(ruleIndex, {
                      tiers: [...rule.tiers, createEmptyTier()],
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
    </Dialog>
  )
}

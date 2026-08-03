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
import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

import type { ChannelNameOption } from '../types'

type ChannelGroupEditorProps = {
  /** Ordered groups; each group holds interchangeable channel ids */
  value: number[][]
  onChange: (groups: number[][]) => void
  channels: ChannelNameOption[]
}

/**
 * Edits the ordered list of channel groups behind one routing rule.
 *
 * Order matters: the relay tries group 1 first and only falls through to the
 * next group when every channel in it fails, so the editor exposes explicit
 * move up/down instead of leaving the order implicit.
 */
export function ChannelGroupEditor({
  value,
  onChange,
  channels,
}: ChannelGroupEditorProps) {
  const { t } = useTranslation()
  const groups = value.length > 0 ? value : [[]]

  const options = channels.map((channel) => ({
    value: String(channel.id),
    label: `${channel.name} (ID: ${channel.id})`,
  }))

  const replaceGroup = (index: number, next: number[]) => {
    onChange(groups.map((group, i) => (i === index ? next : group)))
  }

  const moveGroup = (index: number, direction: -1 | 1) => {
    const target = index + direction
    if (target < 0 || target >= groups.length) return
    const next = [...groups]
    ;[next[index], next[target]] = [next[target], next[index]]
    onChange(next)
  }

  return (
    <div className='space-y-3'>
      {groups.map((group, index) => (
        <div
          // Groups are positional and reorderable; index is their identity.
          // eslint-disable-next-line react/no-array-index-key
          key={index}
          className='space-y-2 rounded-lg border p-3'
        >
          <div className='flex items-center justify-between'>
            <div className='flex items-center gap-2'>
              <Badge variant='secondary'>
                {t('Channel group')} {index + 1}
              </Badge>
              <span className='text-muted-foreground text-xs'>
                {t('{{count}} channels', { count: group.length })}
              </span>
            </div>
            <div className='flex items-center gap-1'>
              <Button
                type='button'
                variant='ghost'
                size='icon'
                className='h-7 w-7'
                aria-label={t('Move up')}
                disabled={index === 0}
                onClick={() => moveGroup(index, -1)}
              >
                <ArrowUp className='h-4 w-4' />
              </Button>
              <Button
                type='button'
                variant='ghost'
                size='icon'
                className='h-7 w-7'
                aria-label={t('Move down')}
                disabled={index === groups.length - 1}
                onClick={() => moveGroup(index, 1)}
              >
                <ArrowDown className='h-4 w-4' />
              </Button>
              <Button
                type='button'
                variant='ghost'
                size='icon'
                className='text-destructive h-7 w-7'
                aria-label={t('Remove group')}
                disabled={groups.length === 1}
                onClick={() =>
                  onChange(groups.filter((_, i) => i !== index))
                }
              >
                <Trash2 className='h-4 w-4' />
              </Button>
            </div>
          </div>
          <MultiSelect
            options={options}
            selected={group.map(String)}
            onChange={(values) =>
              replaceGroup(
                index,
                values.map((id) => Number.parseInt(id, 10)).filter(Number.isFinite)
              )
            }
            placeholder={t('Select channels')}
          />
        </div>
      ))}
      <Button
        type='button'
        variant='outline'
        size='sm'
        onClick={() => onChange([...groups, []])}
      >
        <Plus className='h-4 w-4' />
        {t('Add channel group')}
      </Button>
    </div>
  )
}

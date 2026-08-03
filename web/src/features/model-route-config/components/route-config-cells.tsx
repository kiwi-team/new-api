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
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

/** How many patterns/channels are shown before collapsing into a tooltip. */
const INLINE_LIMIT = 2

type PatternBadgeListProps = {
  label: string
  items: string[]
  variant?: 'default' | 'secondary' | 'outline'
}

/** Compact pattern list; the full set is available on hover. */
export function PatternBadgeList({
  label,
  items,
  variant = 'secondary',
}: PatternBadgeListProps) {
  const { t } = useTranslation()
  if (items.length === 0) return null

  const visible = items.slice(0, INLINE_LIMIT)
  const hiddenCount = items.length - visible.length

  return (
    <Tooltip>
      <TooltipTrigger
        render={<span className='inline-flex flex-wrap items-center gap-1' />}
      >
        <Badge variant='outline'>{label}</Badge>
        {visible.map((item) => (
          <Badge key={item} variant={variant} className='max-w-40 truncate'>
            {item}
          </Badge>
        ))}
        {hiddenCount > 0 && (
          <Badge variant='outline'>+{hiddenCount}</Badge>
        )}
      </TooltipTrigger>
      <TooltipContent>
        <div className='max-w-sm space-y-0.5'>
          <div className='font-medium'>
            {label} ({t('{{count}} items', { count: items.length })})
          </div>
          {items.map((item) => (
            <div key={item} className='break-all'>
              {item}
            </div>
          ))}
        </div>
      </TooltipContent>
    </Tooltip>
  )
}

type ChannelGroupsCellProps = {
  groups: number[][]
  channelNames: Record<number, string>
}

/** Ordered channel groups, collapsed to a count with details on hover. */
export function ChannelGroupsCell({
  groups,
  channelNames,
}: ChannelGroupsCellProps) {
  const { t } = useTranslation()
  if (!groups || groups.length === 0) {
    return <span className='text-muted-foreground'>-</span>
  }

  const channelLabel = (id: number) => channelNames[id] ?? `ID: ${id}`

  return (
    <Tooltip>
      <TooltipTrigger render={<span className='inline-flex gap-1' />}>
        <Badge variant='secondary'>
          {t('{{count}} groups', { count: groups.length })}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>
        <div className='max-w-sm space-y-2'>
          {groups.map((group, index) => (
            // Group order is meaningful and stable within one render.
            // eslint-disable-next-line react/no-array-index-key
            <div key={index}>
              <div className='font-medium'>
                {t('Channel group')} {index + 1} ·{' '}
                {t('{{count}} channels', { count: group.length })}
              </div>
              <div className='text-muted-foreground'>
                {group.map(channelLabel).join(', ') || '-'}
              </div>
            </div>
          ))}
        </div>
      </TooltipContent>
    </Tooltip>
  )
}

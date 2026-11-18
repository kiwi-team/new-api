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

import { LearnMore } from '@/components/learn-more'
import { PopoverTitle } from '@/components/ui/popover'

type Props = {
  field: string
  label: string
  metric: string
  direction: 'increase' | 'decrease'
  mode: string
}

// Content follows the selected metric and direction; examples deliberately name
// their assumed baseline so they cannot be mistaken for the user's saved rule.
export function RuleFieldHelp(props: Props) {
  const { t } = useTranslation()
  const volume = props.metric === 'request_volume_change'
  const traffic = volume || props.metric === 'request_started_count'
  if (props.metric === 'legacy_notice') return null
  let description = ''
  let example = ''
  switch (props.field) {
    case 'hold':
      if (['schedule_report', 'milestone'].includes(props.mode)) return null
      description = t(
        'How long the alert condition must remain true before an incident opens. If it stops matching or data becomes unavailable, timing restarts. Set 0 to trigger on the next matching evaluation.'
      )
      if (volume && props.direction === 'increase') {
        example = t(
          'Example: with a 1-minute window, a baseline of 2 calls and an increase threshold of ≥100%, each evaluation needs at least 4 calls in the last minute. A 60-second hold requires this to stay true for a full minute; a single burst may leave the window too soon.'
        )
      } else {
        example = t(
          'Example: with a 30-second hold, the alert condition must stay true for at least 30 seconds after it is first observed. A matching value followed by a normal value restarts the timer.'
        )
      }
      break
    case 'recoveryHold':
      if (props.mode !== 'incident') return null
      description = t(
        'How long the recovery condition must remain true before the incident is marked resolved. Abnormal or unavailable data restarts timing. Set 0 to resolve on the next matching evaluation. This does not re-enable a disabled channel or API key.'
      )
      example = t(
        'Example: set 20 seconds. The recovery condition must hold for at least 20 seconds after it is first observed. Evaluation and notification delivery can add some delay.'
      )
      break
    case 'recoveryThreshold':
      if (props.mode !== 'incident') return null
      if (volume) {
        description = t(
          'The maximum remaining change allowed for recovery, using the baseline frozen when the alert condition first matched. It must be below the alert threshold. It is not the percentage of traffic regained.'
        )
        if (props.direction === 'increase') {
          example = t(
            'Example: assume a baseline of 200 calls per window. A ≥50% increase alerts at 300 calls. A recovery threshold of 20% requires 240 calls or fewer, maintained for the recovery duration.'
          )
        } else {
          example = t(
            'Example: assume a baseline of 200 calls per window. A ≥50% decrease alerts at 100 calls or fewer. A recovery threshold of 20% requires 160 calls or more, maintained for the recovery duration.'
          )
        }
      } else {
        description = t(
          'For > or ≥ alerts, a value at or below the recovery threshold meets recovery. For < or ≤ alerts, a value at or above it meets recovery. The recovery condition must remain true for the recovery duration.'
        )
        example = t(
          'Example: a success rate below 95% triggers an alert. With a recovery threshold of 99%, recovery requires a success rate of at least 99%.'
        )
      }
      break
    case 'metric':
      if (!traffic) return null
      description = t(
        'Counts calls when they start, including failed and unfinished calls. Channel and channel-model rules count upstream attempts, including retries; models across all channels count each gateway request once.'
      )
      break
    case 'operator':
      if (!volume) return null
      description = t(
        '≥ includes the exact threshold; > requires a larger change. Example: with a baseline of 2 calls and a 100% increase threshold, ≥ matches 4 calls, while > needs at least 5.'
      )
      break
    case 'threshold':
      if (!volume) return null
      description = t(
        'The change in the selected direction needed to trigger. Enter 50 for 50%. A 100% increase doubles the calls; a 100% decrease means no calls. The comparison operator decides whether equality counts.'
      )
      break
    case 'changeDirection':
      if (!volume) return null
      description = t(
        'Increase = (current calls − baseline) ÷ baseline × 100%. Decrease = (baseline − current calls) ÷ baseline × 100%. Calls and baseline use the same statistics window.'
      )
      break
    case 'baselineMode':
      if (!volume) return null
      description = t(
        'Manual baseline uses the expected calls you enter for one statistics window. Historical average calculates that count from an earlier period, excluding the current window. A zero or insufficient historical baseline waits for more data.'
      )
      break
    case 'baselineCount':
      if (!volume) return null
      description = t(
        'The expected calls in one statistics window, independently of the hold duration. With a 1-minute window, 2 means 2 calls per minute. A 100% increase reaches 4 calls per minute.'
      )
      break
    case 'baselineMinutes':
      if (!volume) return null
      description = t(
        'The earlier period used to calculate the historical average. It ends where the current statistics window starts, and must be at least as long as that window.'
      )
      example = t(
        'Example: with a 5-minute statistics window and 30 minutes of history, 120 historical calls become a baseline of 20 calls per 5 minutes. The current 5 minutes are excluded from the history.'
      )
      break
    case 'baselineMinimumCount':
      if (!volume) return null
      description = t(
        'The minimum historical average calls per statistics window needed to evaluate a percentage change. Below this count, the rule waits instead of alerting on sparse traffic. This is not a minimum for current calls; zero current calls can still trigger a decrease.'
      )
      break
    case 'window':
    case 'minutes':
      if (!traffic) return null
      description = t(
        'A rolling window counts calls started in the most recent period and moves at every evaluation. A 1-minute window always means the last 60 seconds, not a calendar minute. The hold duration is an additional requirement and does not extend this window.'
      )
      break
    default:
      return null
  }
  return (
    <LearnMore
      triggerProps={{
        type: 'button',
        'aria-label': `${t('Learn more')}: ${t(props.label)}`,
        className: 'size-5 shrink-0 rounded-full',
      }}
      contentProps={{
        collisionPadding: 16,
        className:
          'w-96 max-w-[calc(100vw-2rem)] max-h-[min(var(--available-height),calc(100dvh-2rem))] gap-3 overflow-y-auto overscroll-contain p-4 text-sm leading-relaxed',
      }}
    >
      <PopoverTitle className='text-foreground'>{t(props.label)}</PopoverTitle>
      <p>{description}</p>
      {example && (
        <div className='bg-muted/50 space-y-1 rounded-md p-3'>
          <p className='text-foreground font-medium'>{t('Example')}</p>
          <p>{example}</p>
        </div>
      )}
    </LearnMore>
  )
}

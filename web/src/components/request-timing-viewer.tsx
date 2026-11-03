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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { EmptyState } from '@/components/empty-state'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import {
  buildTimingSummary,
  timingRecord,
  type TimingRow,
} from './request-timing-viewer/timing-summary'

function readTiming(value: unknown) {
  let parsed: unknown = value
  if (typeof value === 'string') {
    try {
      parsed = JSON.parse(value)
    } catch {
      return { snapshot: undefined, json: value }
    }
  }
  return {
    snapshot: timingRecord(parsed),
    json:
      typeof value !== 'string' && parsed == null
        ? ''
        : (JSON.stringify(parsed, null, 2) ?? ''),
  }
}

/** Shared passive viewer; permissions and fetching remain with each log page. */
export function RequestTimingViewer(props: { data: unknown }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const timing = useMemo(() => readTiming(props.data), [props.data])
  const sections = timing.snapshot ? buildTimingSummary(timing.snapshot, t) : []
  const reasonLabels: Record<string, string> = {
    pending_at_finalize: t('Pending at snapshot'),
    not_observed: t('Not recorded'),
    not_applicable: t('Not applicable'),
    unsupported_transport: t('Unsupported by this transport'),
    ambiguous_transport_retry: t(
      'Multiple writes; duration cannot be paired reliably'
    ),
    early_response: t('Response arrived before upload completed'),
  }
  function displayDuration(row: TimingRow) {
    if (row.value === undefined) {
      return reasonLabels[row.reason ?? ''] ?? t('Not recorded')
    }
    if (row.value > 0 && row.value < 0.01) {
      return t('< {{duration}} ms', { duration: formatNumber(0.01, locale) })
    }
    if (row.value < 1000) {
      return t('{{duration}} ms', { duration: formatNumber(row.value, locale) })
    }
    return t('{{duration}} s', {
      duration: formatNumber(row.value / 1000, locale),
    })
  }
  return (
    <div role='group' aria-label={t('Request timing')} className='min-w-0'>
      <Tabs defaultValue='summary'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <TabsList aria-label={t('Timing display mode')}>
            <TabsTrigger value='summary'>{t('Stage summary')}</TabsTrigger>
            <TabsTrigger value='json'>JSON</TabsTrigger>
          </TabsList>
          {timing.json && (
            <CopyButton
              value={timing.json}
              size='sm'
              tooltip={t('Copy JSON')}
            />
          )}
        </div>
        <TabsContent value='summary'>
          {sections.length ? (
            <div
              className='max-h-[28rem] space-y-4 overflow-auto p-1 text-xs'
              tabIndex={0}
            >
              <p className='text-muted-foreground'>
                {t(
                  'This is a timing snapshot. Pending stages have no final duration.'
                )}
              </p>
              <p className='text-muted-foreground'>
                {t(
                  'Streaming stages overlap; do not add these durations. Header waiting includes network and upstream processing time.'
                )}
              </p>
              {timing.snapshot?.truncated === true && (
                <p className='text-muted-foreground'>
                  {t(
                    'Some timing details were truncated; the summary may be incomplete.'
                  )}
                </p>
              )}
              {sections.map((section) => (
                <section
                  key={section.title}
                  aria-label={section.title}
                  className='min-w-0 space-y-1.5'
                >
                  <h4 className='font-medium break-words'>{section.title}</h4>
                  {section.description && (
                    <p className='text-muted-foreground break-all'>
                      {section.description}
                    </p>
                  )}
                  <dl className='space-y-1.5'>
                    {section.rows.map((row) => (
                      <div
                        key={row.id ?? row.label}
                        className='flex min-w-0 flex-wrap justify-between gap-x-3 gap-y-1'
                      >
                        <dt className='text-muted-foreground'>{row.label}</dt>
                        <dd className='min-w-0 text-right break-words tabular-nums'>
                          {displayDuration(row)}
                          {row.note ? ` · ${row.note}` : ''}
                        </dd>
                      </div>
                    ))}
                  </dl>
                </section>
              ))}
              {timingRecord(timing.snapshot?.downstream) &&
                timingRecord(timing.snapshot?.coverage)?.downstream !==
                  'not_applicable' && (
                  <p className='text-muted-foreground'>
                    {t(
                      'Downstream times measure local writes and flushes, not confirmation that the client received the data.'
                    )}
                  </p>
                )}
            </div>
          ) : (
            <EmptyState
              className='min-h-0 py-4'
              title={t('No timing summary available')}
              description={t('Use JSON to inspect any original timing data.')}
            />
          )}
        </TabsContent>
        <TabsContent value='json'>
          <pre
            tabIndex={0}
            className='bg-muted/40 max-h-80 max-w-full overflow-auto rounded-md p-3 font-mono text-xs whitespace-pre-wrap'
          >
            {timing.json || '-'}
          </pre>
        </TabsContent>
      </Tabs>
    </div>
  )
}

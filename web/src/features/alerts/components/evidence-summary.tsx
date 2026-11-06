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
import { z } from 'zod'

import { StaticDataTable } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { toIntlLocale } from '@/i18n/languages'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatNumber } from '@/lib/format'

import { alertLabel } from '../lib/labels'
import type { RuleSpec } from '../types'

const quantity = z.string().regex(/^-?[0-9]+$/)
const usage = z.object({
  token_id: z.number(),
  token_name: z.string(),
  quota: quantity,
  requests: quantity,
  input_tokens: quantity,
  output_tokens: quantity,
  cached_tokens: quantity,
  cache_write_5m_tokens: quantity,
  cache_write_1h_tokens: quantity,
})
const evidenceSchema = z.object({
  measurement: z.object({
    value: quantity,
    samples: quantity,
    status: z.string(),
    reason: z.string().optional(),
    window_start: z.number(),
    window_end: z.number(),
    request_ids: z.array(z.string()).nullable(),
    requests: z
      .array(
        z.object({
          kind: z.string(),
          request_id: z.string(),
          channel_name: z.string(),
          model: z.string(),
          started_at: z.number(),
          last_output_at: z.number(),
          finished_at: z.number(),
          duration_ms: z.number().nullable().optional(),
          ttft_ms: z.number().nullable().optional(),
        })
      )
      .optional(),
    report: z
      .object({
        totals: usage,
        keys: z.array(usage),
        backfill: z.boolean(),
        first_partial: z.boolean(),
        source_status: z.string(),
        source_reason: z.string(),
      })
      .optional(),
  }),
})

// Large int64 counters stay exact. The shared formatter handles ordinary values;
// counters outside JavaScript's safe range retain the server's decimal text.
export function EvidenceSummary(props: { evidence: string; config: RuleSpec }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const date = new Intl.DateTimeFormat(locale, {
    dateStyle: 'short',
    timeStyle: 'medium',
  })
  let decoded: unknown
  try {
    decoded = JSON.parse(props.evidence)
  } catch {
    return null
  }
  if (props.config.condition.metric === 'legacy_notice') {
    const notice = z
      .object({ content: z.string().max(16384) })
      .safeParse(decoded)
    if (!notice.success) return null
    return (
      <div className='space-y-2 text-sm' role='note'>
        <p className='text-muted-foreground'>{t('Legacy trigger retained')}</p>
        <p className='break-words whitespace-pre-wrap'>{notice.data.content}</p>
      </div>
    )
  }
  const parsed = evidenceSchema.safeParse(decoded)
  if (!parsed.success) return null
  const m = parsed.data.measurement
  const number = (value: string) =>
    Number.isSafeInteger(Number(value))
      ? formatNumber(Number(value), locale)
      : value
  const amount = (value: string) => {
    const unit = props.config.quota_unit || 500000
    if (Number.isSafeInteger(Number(value))) {
      return formatCurrencyFromUSD(Number(value) / unit, {
        digitsLarge: 4,
        digitsSmall: 8,
        locale,
      })
    }
    const q = BigInt(value),
      u = BigInt(unit),
      sign = q < 0n ? '-' : '',
      positive = q < 0n ? -q : q
    const fractional = (((positive % u) * 100000000n) / u)
      .toString()
      .padStart(8, '0')
      .replace(/0+$/, '')
    return `${sign}${positive / u}${fractional ? `.${fractional}` : ''} USD`
  }
  const value = ['spend_net', 'balance_remaining'].includes(
    props.config.condition.metric
  )
    ? amount(m.value)
    : number(m.value)
  return (
    <section className='space-y-3 text-sm'>
      <div className='flex flex-wrap items-center gap-3'>
        <Badge variant='outline'>{alertLabel(m.status, t)}</Badge>
        {m.reason && <span>{alertLabel(m.reason, t)}</span>}
        <span>
          {alertLabel(props.config.condition.metric, t)}: {value}
        </span>
        <span>
          {t('Valid samples')}: {number(m.samples)}
        </span>
      </div>
      <p>
        {new Intl.DateTimeFormat(locale, {
          dateStyle: 'short',
          timeStyle: 'medium',
        }).format(m.window_start)}{' '}
        –{' '}
        {new Intl.DateTimeFormat(locale, {
          dateStyle: 'short',
          timeStyle: 'medium',
        }).format(m.window_end)}
      </p>
      {(m.request_ids ?? []).length > 0 && (
        <p className='break-all'>
          {t('Example request IDs')}: {m.request_ids?.join(', ')}
        </p>
      )}
      {!!m.requests?.length && (
        <StaticDataTable
          data={m.requests}
          getRowKey={(row, index) => `${row.kind}:${row.request_id}:${index}`}
          columns={[
            {
              id: 'kind',
              header: t('Sample category'),
              cell: (row) => alertLabel(row.kind, t),
            },
            {
              id: 'channel',
              header: t('Monitored object'),
              cell: (row) => (
                <div>
                  {row.channel_name || t('Gateway request')}
                  <p className='text-muted-foreground text-xs'>{row.model}</p>
                </div>
              ),
            },
            {
              id: 'request',
              header: t('Request ID'),
              cell: (row) => (
                <span className='break-all'>{row.request_id || '—'}</span>
              ),
            },
            {
              id: 'started',
              header: t('Start Time'),
              cell: (row) =>
                row.started_at > 0 ? date.format(row.started_at) : '—',
            },
            {
              id: 'progress',
              header: t('Last effective output'),
              cell: (row) =>
                row.last_output_at > 0 ? date.format(row.last_output_at) : '—',
            },
            {
              id: 'duration',
              header: t('Total duration'),
              cell: (row) =>
                row.duration_ms == null
                  ? '—'
                  : `${number(String(row.duration_ms))} ms`,
            },
            {
              id: 'ttft',
              header: t('First output latency'),
              cell: (row) =>
                row.ttft_ms == null ? '—' : `${number(String(row.ttft_ms))} ms`,
            },
          ]}
        />
      )}
      {m.report && (
        <>
          {(m.report.backfill || m.report.first_partial) && (
            <p>
              {t(
                m.report.backfill ? 'Backfilled report' : 'First partial period'
              )}
            </p>
          )}
          <StaticDataTable
            data={[
              { ...m.report.totals, token_name: t('Total') },
              ...m.report.keys,
            ]}
            getRowKey={(row, index) => `${index}:${row.token_id}`}
            columns={[
              {
                id: 'key',
                header: t('API Keys'),
                cell: (row) =>
                  row.token_name ||
                  (row.token_id === 0 ? t('No API key') : t('Deleted API key')),
              },
              {
                id: 'amount',
                header: t('Net spending'),
                cell: (row) => amount(row.quota),
              },
              {
                id: 'requests',
                header: t('Request count'),
                cell: (row) => number(row.requests),
              },
              {
                id: 'input',
                header: t('Input tokens'),
                cell: (row) => number(row.input_tokens),
              },
              {
                id: 'output',
                header: t('Output tokens'),
                cell: (row) => number(row.output_tokens),
              },
              {
                id: 'cache',
                header: t('Cache read tokens'),
                cell: (row) => number(row.cached_tokens),
              },
            ]}
          />
        </>
      )}
    </section>
  )
}

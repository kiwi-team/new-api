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
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { formatTimestampToDate } from '@/lib/format'

import { getErrorLogBody, getErrorLogHeader } from '../api'
import { formatUseTime, parseErrorLogExtra } from '../lib'
import type { ErrorLog } from '../types'

type ErrorLogDetailSheetProps = {
  log: ErrorLog | null
  onOpenChange: (open: boolean) => void
  /** Root sees request headers, which may carry upstream credentials */
  canViewHeader: boolean
}

/** Correlation ids are meant to be pasted into other tools. */
function IdWithCopy({ value }: { value: string }) {
  return (
    <span className='inline-flex items-center gap-1'>
      <span className='font-mono text-xs'>{value}</span>
      <CopyButton value={value} className='h-5 w-5' />
    </span>
  )
}

function Field(props: { label: string; value: React.ReactNode }) {
  return (
    <div className='space-y-0.5'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='text-sm break-all'>{props.value}</div>
    </div>
  )
}

/**
 * Renders a raw API payload. The value is typed `unknown` on purpose: it comes
 * straight from an untyped JSON response, and handing React a non-string here
 * throws during render, which the root error boundary turns into a full-page
 * 500 instead of a broken field.
 */
function JsonBlock(props: { value: unknown }) {
  let text =
    typeof props.value === 'string' ? props.value : JSON.stringify(props.value)
  try {
    text = JSON.stringify(JSON.parse(text), null, 2)
  } catch {
    // Not JSON — show as-is.
  }
  return (
    <pre className='bg-muted/40 max-h-80 overflow-auto rounded-md p-3 text-xs whitespace-pre-wrap'>
      {text}
    </pre>
  )
}

export function ErrorLogDetailSheet({
  log,
  onOpenChange,
  canViewHeader,
}: ErrorLogDetailSheetProps) {
  const { t } = useTranslation()
  const logId = log?.id

  // Body and headers are heavy, so they load only once the row is opened.
  const bodyQuery = useQuery({
    queryKey: ['error-logs', 'body', logId],
    queryFn: () => getErrorLogBody(logId as number),
    enabled: logId !== undefined,
  })
  const headerQuery = useQuery({
    queryKey: ['error-logs', 'header', logId],
    queryFn: () => getErrorLogHeader(logId as number),
    enabled: logId !== undefined && canViewHeader,
  })

  if (!log) return null

  const extra = parseErrorLogExtra(log.extra)

  return (
    <Sheet open onOpenChange={onOpenChange}>
      <SheetContent className='w-full sm:max-w-2xl'>
        <SheetHeader>
          <SheetTitle>{t('Error details')}</SheetTitle>
          <SheetDescription>
            #{log.id} · {formatTimestampToDate(log.created_at)}
          </SheetDescription>
        </SheetHeader>

        <div className='space-y-5 overflow-y-auto px-4 pb-4'>
          <div className='grid grid-cols-2 gap-3'>
            <Field label={t('Model name')} value={log.model_name || '-'} />
            <Field
              label={t('Channel')}
              value={`${log.channel_name || '-'} (#${log.channel_id})`}
            />
            <Field
              label={t('Key')}
              value={`${log.token_name || '-'} (#${log.token_id})`}
            />
            <Field label={t('User ID')} value={log.user_id} />
            <Field label={t('Status code')} value={log.status_code || '-'} />
            <Field label={t('Latency')} value={formatUseTime(log.use_time_ms)} />
            <Field
              label={t('Request ID')}
              value={
                log.request_id ? (
                  <IdWithCopy value={log.request_id} />
                ) : (
                  '-'
                )
              }
            />
            <Field label={t('IP')} value={log.ip || '-'} />
            <Field
              label={t('Client UID')}
              value={log.client_user_id || '-'}
            />
            <Field label={t('Scenario')} value={log.client_scenairo || '-'} />
            <Field
              label={t('Session ID')}
              value={
                log.session_id ? <IdWithCopy value={log.session_id} /> : '-'
              }
            />
            <Field label={t('Error type')} value={log.type || '-'} />
            {extra.mt_session_id && (
              <Field
                label={t('MT Session ID')}
                value={<IdWithCopy value={String(extra.mt_session_id)} />}
              />
            )}
            {extra.trace_id && (
              <Field
                label={t('Trace ID')}
                value={<IdWithCopy value={String(extra.trace_id)} />}
              />
            )}
            {extra.traj_id && (
              <Field
                label={t('Traj ID')}
                value={<IdWithCopy value={String(extra.traj_id)} />}
              />
            )}
          </div>

          <div className='space-y-2'>
            <h3 className='text-sm font-medium'>{t('Message')}</h3>
            <p className='bg-muted/40 rounded-md p-3 text-sm break-all'>
              {log.message || '-'}
            </p>
            {log.param && (
              <p className='text-muted-foreground text-xs break-all'>
                {t('param')}: {log.param}
              </p>
            )}
          </div>

          <div className='space-y-2'>
            <h3 className='text-sm font-medium'>{t('Request body')}</h3>
            {bodyQuery.isLoading ? (
              <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>
            ) : (
              <JsonBlock value={bodyQuery.data?.data || '-'} />
            )}
          </div>

          {canViewHeader && (
            <div className='space-y-2'>
              <h3 className='text-sm font-medium'>{t('Request headers')}</h3>
              {headerQuery.isLoading ? (
                <p className='text-muted-foreground text-sm'>
                  {t('Loading...')}
                </p>
              ) : (
                <JsonBlock value={headerQuery.data?.data?.content || '-'} />
              )}
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}

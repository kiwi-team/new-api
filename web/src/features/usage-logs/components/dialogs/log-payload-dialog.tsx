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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { getLogHeaders, getLogRequestBody, getLogResponseBody } from '../../api'

type PayloadTab = 'request' | 'response' | 'header'

type LogPayloadDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  logId: number
}

const LOADERS: Record<PayloadTab, (id: number) => Promise<string>> = {
  request: getLogRequestBody,
  response: getLogResponseBody,
  header: getLogHeaders,
}

function prettyPrint(value: string): string {
  if (!value) return ''
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    // Streamed responses are SSE frames rather than a single JSON document.
    return value
  }
}

function PayloadBody(props: {
  isLoading: boolean
  error: Error | null
  content: string
}) {
  const { t } = useTranslation()

  if (props.isLoading) {
    return (
      <p className='text-muted-foreground py-8 text-center text-sm'>
        {t('Loading...')}
      </p>
    )
  }
  if (props.error) {
    return (
      <p className='text-destructive py-8 text-center text-sm'>
        {props.error.message}
      </p>
    )
  }
  if (!props.content) {
    return (
      <p className='text-muted-foreground py-8 text-center text-sm'>
        {t('This log has no stored content for this section')}
      </p>
    )
  }
  return (
    <pre className='bg-muted/40 max-h-[60vh] overflow-auto rounded-md p-3 text-xs whitespace-pre-wrap'>
      {props.content}
    </pre>
  )
}

/**
 * Raw request/response/header viewer for one log entry.
 *
 * Root only — the caller gates the entry point, and the endpoints themselves
 * sit behind `RootAuth`. Each tab loads lazily so opening the dialog does not
 * pull megabytes of prompt text the user may not need.
 */
export function LogPayloadDialog({
  open,
  onOpenChange,
  logId,
}: LogPayloadDialogProps) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<PayloadTab>('request')

  const { data, isLoading, error } = useQuery({
    queryKey: ['usage-logs', 'payload', logId, tab],
    queryFn: () => LOADERS[tab](logId),
    enabled: open,
    retry: false,
  })

  const content = prettyPrint(data ?? '')

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Raw payload')}
      description={`#${logId}`}
      contentClassName='sm:max-w-3xl'
      contentHeight='auto'
    >
      <div className='space-y-3'>
        <div className='flex items-center justify-between gap-2'>
          <Tabs value={tab} onValueChange={(value) => setTab(value as PayloadTab)}>
            <TabsList>
              <TabsTrigger value='request'>{t('Request body')}</TabsTrigger>
              <TabsTrigger value='response'>{t('Response body')}</TabsTrigger>
              <TabsTrigger value='header'>{t('Request headers')}</TabsTrigger>
            </TabsList>
          </Tabs>
          {content && <CopyButton value={content} />}
        </div>

        <PayloadBody
          isLoading={isLoading}
          error={error as Error | null}
          content={content}
        />
      </div>
    </Dialog>
  )
}

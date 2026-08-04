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
import { getRouteApi } from '@tanstack/react-router'
import { Download, Search } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table'
import { DateTimePicker } from '@/components/datetime-picker'
import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Pagination,
  PaginationContent,
  PaginationItem,
} from '@/components/ui/pagination'
import { formatTimestampToDate } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { exportErrorLogsCsv, getErrorLogs } from './api'
import { ErrorLogDetailSheet } from './components/error-log-detail-sheet'
import {
  EXPORT_MAX_RANGE_HOURS,
  errorLogRowKey,
  formatUseTime,
  isRangeExportable,
  statusTone,
} from './lib'
import type { ErrorLog, ErrorLogFilters } from './types'

const route = getRouteApi('/_authenticated/error-logs/')
const PAGE_SIZE = 20

function hoursAgo(hours: number): Date {
  return new Date(Date.now() - hours * 3_600_000)
}

export function ErrorLogsPage() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )

  const page = search.page ?? 1
  const [startTime, setStartTime] = useState<Date | undefined>(() => hoursAgo(24))
  const [endTime, setEndTime] = useState<Date | undefined>(() => new Date())
  const [modelName, setModelName] = useState('')
  const [requestId, setRequestId] = useState('')
  const [channelId, setChannelId] = useState('')
  const [tokenId, setTokenId] = useState('')
  const [clientUserId, setClientUserId] = useState('')
  const [mtSessionId, setMtSessionId] = useState('')
  const [traceId, setTraceId] = useState('')
  const [trajId, setTrajId] = useState('')
  const [sessionId, setSessionId] = useState('')
  const [selected, setSelected] = useState<ErrorLog | null>(null)

  // Filters apply on submit so typing an id does not fire a query per key.
  const [filters, setFilters] = useState<ErrorLogFilters>(() => ({
    start_timestamp: Math.floor(hoursAgo(24).getTime() / 1000),
    end_timestamp: Math.floor(Date.now() / 1000),
  }))

  const logsQuery = useQuery({
    queryKey: ['error-logs', 'list', page, filters],
    queryFn: async () => {
      const result = await getErrorLogs({ ...filters, p: page, page_size: PAGE_SIZE })
      if (!result.success) {
        toast.error(result.message || t('Failed to load'))
        return { items: [], total: 0 }
      }
      return {
        items: result.data?.items ?? [],
        total: result.data?.total ?? 0,
      }
    },
    placeholderData: (previous) => previous,
  })

  const buildFilters = (): ErrorLogFilters => ({
    start_timestamp: startTime ? Math.floor(startTime.getTime() / 1000) : undefined,
    end_timestamp: endTime ? Math.floor(endTime.getTime() / 1000) : undefined,
    model_name: modelName.trim(),
    request_id: requestId.trim(),
    channel: Number.parseInt(channelId, 10) || undefined,
    token_id: Number.parseInt(tokenId, 10) || undefined,
    client_user_id: clientUserId.trim(),
    // Correlation ids live inside the `extra` JSON blob; the backend trims
    // them server-side too, since they are usually pasted from other tools.
    mt_session_id: mtSessionId.trim(),
    trace_id: traceId.trim(),
    traj_id: trajId.trim(),
    session_id: sessionId.trim(),
  })

  const handleSearch = () => {
    setFilters(buildFilters())
    void navigate({ search: { page: 1 } })
  }

  const handleExport = async () => {
    if (!isRangeExportable(startTime, endTime)) {
      toast.warning(
        t('Export is limited to a {{hours}} hour range', {
          hours: EXPORT_MAX_RANGE_HOURS,
        })
      )
      return
    }
    try {
      const blob = await exportErrorLogsCsv(buildFilters())
      const url = window.URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = 'error_logs.csv'
      link.click()
      window.URL.revokeObjectURL(url)
    } catch {
      toast.error(t('Export failed'))
    }
  }

  const total = logsQuery.data?.total ?? 0
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('Error Logs')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          {isRoot && (
            <Button
              variant='outline'
              size='sm'
              onClick={() => void handleExport()}
            >
              <Download className='h-4 w-4' />
              {t('Export CSV')}
            </Button>
          )}
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex h-full min-h-0 flex-col gap-4'>
            <div className='flex flex-wrap items-end gap-3'>
              <div className='grid gap-1.5'>
                <Label>{t('Start Time')}</Label>
                <DateTimePicker value={startTime} onChange={setStartTime} />
              </div>
              <div className='grid gap-1.5'>
                <Label>{t('End Time')}</Label>
                <DateTimePicker value={endTime} onChange={setEndTime} />
              </div>
              <div className='grid gap-1.5'>
                <Label htmlFor='el-model'>{t('Model name')}</Label>
                <Input
                  id='el-model'
                  className='w-40'
                  value={modelName}
                  onChange={(event) => setModelName(event.target.value)}
                />
              </div>
              <div className='grid gap-1.5'>
                <Label htmlFor='el-request'>{t('Request ID')}</Label>
                <Input
                  id='el-request'
                  className='w-48'
                  value={requestId}
                  onChange={(event) => setRequestId(event.target.value)}
                />
              </div>
              <div className='grid gap-1.5'>
                <Label htmlFor='el-channel'>{t('Channel ID')}</Label>
                <Input
                  id='el-channel'
                  className='w-28'
                  inputMode='numeric'
                  value={channelId}
                  onChange={(event) => setChannelId(event.target.value)}
                />
              </div>
              <div className='grid gap-1.5'>
                <Label htmlFor='el-token'>{t('Key ID')}</Label>
                <Input
                  id='el-token'
                  className='w-28'
                  inputMode='numeric'
                  value={tokenId}
                  onChange={(event) => setTokenId(event.target.value)}
                />
              </div>
              <div className='grid gap-1.5'>
                <Label htmlFor='el-uid'>{t('Client UID')}</Label>
                <Input
                  id='el-uid'
                  className='w-40'
                  value={clientUserId}
                  onChange={(event) => setClientUserId(event.target.value)}
                />
              </div>
              <div className='grid gap-1.5'>
                <Label htmlFor='el-mt-session'>{t('MT Session ID')}</Label>
                <Input
                  id='el-mt-session'
                  className='w-48'
                  value={mtSessionId}
                  onChange={(event) => setMtSessionId(event.target.value)}
                />
              </div>
              <div className='grid gap-1.5'>
                <Label htmlFor='el-trace'>{t('Trace ID')}</Label>
                <Input
                  id='el-trace'
                  className='w-48'
                  value={traceId}
                  onChange={(event) => setTraceId(event.target.value)}
                />
              </div>
              <div className='grid gap-1.5'>
                <Label htmlFor='el-traj'>{t('Traj ID')}</Label>
                <Input
                  id='el-traj'
                  className='w-48'
                  value={trajId}
                  onChange={(event) => setTrajId(event.target.value)}
                />
              </div>
              <div className='grid gap-1.5'>
                <Label htmlFor='el-session'>{t('Session ID')}</Label>
                <Input
                  id='el-session'
                  className='w-48'
                  value={sessionId}
                  onChange={(event) => setSessionId(event.target.value)}
                />
              </div>
              <Button
                className='mb-0.5'
                size='sm'
                onClick={handleSearch}
                disabled={logsQuery.isFetching}
              >
                <Search className='h-4 w-4' />
                {t('Query')}
              </Button>
            </div>

            <div className='min-h-0 flex-1 overflow-auto'>
              <StaticDataTable
                tableClassName='min-w-max'
                data={logsQuery.data?.items ?? []}
                getRowKey={errorLogRowKey}
                emptyContent={t('No error logs for this range')}
                emptyClassName='text-muted-foreground py-8'
                columns={[
                  { id: 'id', header: t('ID'), cell: (log) => log.id },
                  {
                    id: 'time',
                    header: t('Time'),
                    cell: (log) => formatTimestampToDate(log.created_at),
                  },
                  {
                    id: 'model',
                    header: t('Model name'),
                    cellClassName: 'font-medium',
                    cell: (log) => log.model_name || '-',
                  },
                  {
                    id: 'channel',
                    header: t('Channel'),
                    cell: (log) =>
                      log.channel_name ? `${log.channel_name} (#${log.channel_id})` : '-',
                  },
                  {
                    id: 'token',
                    header: t('Key'),
                    cell: (log) => log.token_name || '-',
                  },
                  {
                    id: 'status',
                    header: t('Status code'),
                    cell: (log) =>
                      log.status_code ? (
                        <StatusBadge
                          label={String(log.status_code)}
                          variant={statusTone(log.status_code)}
                          size='sm'
                          copyable={false}
                        />
                      ) : (
                        '-'
                      ),
                  },
                  {
                    id: 'message',
                    header: t('Message'),
                    cell: (log) => (
                      <span className='block max-w-md truncate'>
                        {log.message || '-'}
                      </span>
                    ),
                  },
                  {
                    id: 'latency',
                    header: t('Latency'),
                    cell: (log) => formatUseTime(log.use_time_ms),
                  },
                  {
                    id: 'client-uid',
                    header: t('Client UID'),
                    cell: (log) => log.client_user_id || '-',
                  },
                  {
                    id: 'request-id',
                    header: t('Request ID'),
                    cell: (log) =>
                      log.request_id ? (
                        <span className='block max-w-48 truncate font-mono text-xs'>
                          {log.request_id}
                        </span>
                      ) : (
                        '-'
                      ),
                  },
                  {
                    id: 'actions',
                    header: t('Actions'),
                    cell: (log) => (
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={() => setSelected(log)}
                      >
                        {t('Details')}
                      </Button>
                    ),
                  },
                ]}
              />
            </div>

            <div className='flex items-center justify-between'>
              <span className='text-muted-foreground text-sm'>
                {t('Total')}: {total}
              </span>
              <Pagination>
                <PaginationContent>
                  <PaginationItem>
                    <Button
                      variant='outline'
                      size='sm'
                      disabled={page <= 1}
                      onClick={() => void navigate({ search: { page: page - 1 } })}
                    >
                      {t('Previous')}
                    </Button>
                  </PaginationItem>
                  <PaginationItem>
                    <span className='px-3 text-sm'>
                      {page} / {pageCount}
                    </span>
                  </PaginationItem>
                  <PaginationItem>
                    <Button
                      variant='outline'
                      size='sm'
                      disabled={page >= pageCount}
                      onClick={() => void navigate({ search: { page: page + 1 } })}
                    >
                      {t('Next')}
                    </Button>
                  </PaginationItem>
                </PaginationContent>
              </Pagination>
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ErrorLogDetailSheet
        log={selected}
        onOpenChange={(open) => !open && setSelected(null)}
        canViewHeader={isRoot}
      />
    </>
  )
}

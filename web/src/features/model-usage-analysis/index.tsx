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
import {
  Database,
  DollarSign,
  Download,
  Pencil,
  RefreshCw,
  TrendingUp,
  WalletCards,
} from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table'
import { SectionPageLayout } from '@/components/layout'
import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useDebounce } from '@/hooks/use-debounce'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getModelUsageAnalysis, getUsageUserOptions } from './api'
import {
  buildUsageSummary,
  exportUsageRowsCsv,
  formatCount,
  formatMs,
  formatPercent,
  formatUsd,
  getDefaultUsageDateRange,
  requestRatio,
  usageDateRangeTimestamps,
  usageRowKey,
} from './lib'
import type { ModelUsageRow } from './types'
import { UsageAdjustmentDialog } from './usage-adjustment-dialog'

const DEFAULT_DATE_RANGE = getDefaultUsageDateRange()
const ALL_USERS_VALUE = 'all'
type UsageAnalysisQuery = ReturnType<typeof usageDateRangeTimestamps> & {
  user_id?: number
}

function SummaryCard(props: {
  label: string
  value: string
  hint: string
  icon: ReactNode
}) {
  return (
    <Card>
      <CardContent className='space-y-1 p-4'>
        <div className='text-muted-foreground flex items-center justify-between text-sm'>
          <span>{props.label}</span>
          {props.icon}
        </div>
        <div className='text-lg font-semibold'>{props.value}</div>
        <p className='text-muted-foreground text-xs'>{props.hint}</p>
      </CardContent>
    </Card>
  )
}

export function ModelUsageAnalysisPage() {
  const { t } = useTranslation()
  const role = useAuthStore((state) => state.auth.user?.role) ?? ROLE.GUEST
  const canAdjustUsage = role >= ROLE.ADMIN
  const [startDate, setStartDate] = useState(DEFAULT_DATE_RANGE.start)
  const [endDate, setEndDate] = useState(DEFAULT_DATE_RANGE.end)
  const [selectedUserId, setSelectedUserId] = useState<number | undefined>()
  const [userKeyword, setUserKeyword] = useState('')
  const debouncedUserKeyword = useDebounce(userKeyword, 300)
  const [tokenFilter, setTokenFilter] = useState<string[]>([])
  const [range, setRange] = useState<UsageAnalysisQuery>(() =>
    usageDateRangeTimestamps(DEFAULT_DATE_RANGE.start, DEFAULT_DATE_RANGE.end)
  )
  const [adjustmentRow, setAdjustmentRow] = useState<ModelUsageRow | null>(null)

  const usageQuery = useQuery({
    queryKey: ['model-usage-analysis', range],
    queryFn: () => getModelUsageAnalysis(range),
  })

  const userOptionsQuery = useQuery({
    queryKey: ['model-usage-analysis', 'user-options', debouncedUserKeyword],
    queryFn: () => getUsageUserOptions(debouncedUserKeyword),
    enabled: canAdjustUsage,
  })

  const userSelectItems = useMemo(
    () => [
      { value: ALL_USERS_VALUE, label: t('All users') },
      ...(userOptionsQuery.data ?? []).map((option) => ({
        value: String(option.value),
        label: option.label,
      })),
    ],
    [t, userOptionsQuery.data]
  )

  const rawRows = useMemo(() => usageQuery.data ?? [], [usageQuery.data])

  // The key filter narrows the loaded rows client-side; the endpoint only
  // takes a time range.
  const tokenOptions = useMemo(
    () =>
      [...new Set(rawRows.map((row) => row.token_name).filter(Boolean))].map(
        (name) => ({ value: name as string, label: name as string })
      ),
    [rawRows]
  )

  const rows = useMemo(() => {
    if (tokenFilter.length === 0) return rawRows
    const selected = new Set(tokenFilter)
    return rawRows.filter((row) => selected.has(row.token_name ?? ''))
  }, [rawRows, tokenFilter])

  const summary = useMemo(() => buildUsageSummary(rows), [rows])
  const tokenCount = new Set(rows.map((row) => row.token_name)).size

  const handleRefresh = () => {
    if (!startDate || !endDate) {
      toast.warning(t('Please select a date range'))
      return
    }
    if (startDate > endDate) {
      toast.warning(t('The start date cannot be later than the end date'))
      return
    }
    const next = {
      ...usageDateRangeTimestamps(startDate, endDate),
      ...(selectedUserId === undefined ? {} : { user_id: selectedUserId }),
    }
    // React Query hashes the key structurally, so re-setting an equal range
    // yields the same query and never refetches. Refresh with unchanged dates
    // has to go through refetch() explicitly to bypass staleTime.
    if (
      next.start_timestamp === range.start_timestamp &&
      next.end_timestamp === range.end_timestamp &&
      next.user_id === range.user_id
    ) {
      void usageQuery.refetch()
      return
    }
    setRange(next)
  }

  const handleUserChange = (value: string | null) => {
    const nextUserId =
      !value || value === ALL_USERS_VALUE ? undefined : Number(value)
    setSelectedUserId(nextUserId)
    setTokenFilter([])
    setRange((current) => ({
      start_timestamp: current.start_timestamp,
      end_timestamp: current.end_timestamp,
      ...(nextUserId === undefined ? {} : { user_id: nextUserId }),
    }))
  }

  const handleExport = () => {
    if (rows.length === 0) {
      toast.warning(t('There is no data to export'))
      return
    }
    exportUsageRowsCsv(
      rows,
      {
        start: startDate,
        end: endDate,
      },
      t,
      canAdjustUsage
    )
  }

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>
        {t('Model Usage Analysis')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-4'>
          <div className='flex flex-wrap items-end gap-3'>
            {canAdjustUsage && (
              <div className='grid gap-1.5'>
                <Label htmlFor='model-usage-user-filter'>{t('User')}</Label>
                <Select
                  items={userSelectItems}
                  value={
                    selectedUserId === undefined
                      ? ALL_USERS_VALUE
                      : String(selectedUserId)
                  }
                  onValueChange={handleUserChange}
                >
                  <SelectTrigger
                    id='model-usage-user-filter'
                    className='w-60'
                    aria-label={t('Filter by user')}
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <div className='p-1'>
                      <Input
                        placeholder={t('Filter by user')}
                        value={userKeyword}
                        onChange={(event) => setUserKeyword(event.target.value)}
                      />
                    </div>
                    <SelectGroup>
                      {userSelectItems.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>
            )}
            <div className='grid gap-1.5'>
              <Label htmlFor='model-usage-start-date'>
                {t('Usage start date')}
              </Label>
              <Input
                id='model-usage-start-date'
                type='date'
                value={startDate}
                onChange={(event) => setStartDate(event.target.value)}
              />
            </div>
            <div className='grid gap-1.5'>
              <Label htmlFor='model-usage-end-date'>
                {t('Usage end date')}
              </Label>
              <Input
                id='model-usage-end-date'
                type='date'
                value={endDate}
                onChange={(event) => setEndDate(event.target.value)}
              />
            </div>
            <div className='grid gap-1.5'>
              <Label>{t('Key name')}</Label>
              <MultiSelect
                className='w-64'
                options={tokenOptions}
                selected={tokenFilter}
                onChange={setTokenFilter}
                placeholder={t('All key names')}
                maxVisibleChips={2}
              />
            </div>
            <Button
              className='mb-0.5'
              size='sm'
              onClick={handleRefresh}
              disabled={usageQuery.isFetching}
            >
              <RefreshCw className='h-4 w-4' />
              {t('Refresh')}
            </Button>
            <Button
              className='mb-0.5'
              variant='outline'
              size='sm'
              onClick={handleExport}
              disabled={usageQuery.isFetching || rows.length === 0}
            >
              <Download className='h-4 w-4' />
              {t('Export CSV')}
            </Button>
          </div>

          <div className='grid grid-cols-2 gap-3 lg:grid-cols-5'>
            <SummaryCard
              label={t('Total consumption (USD)')}
              value={formatUsd(summary.costUsd)}
              hint={t('{{keys}} key names, {{rows}} rows', {
                keys: tokenCount,
                rows: rows.length,
              })}
              icon={<DollarSign className='h-4 w-4' />}
            />
            <SummaryCard
              label={t('Total requests')}
              value={formatCount(summary.totalRequests)}
              hint={t('Requests in the current filter')}
              icon={<TrendingUp className='h-4 w-4' />}
            />
            <SummaryCard
              label={t('Cache write')}
              value={formatPercent(
                requestRatio(summary.cacheWriteRequests, summary.totalRequests)
              )}
              hint={`${formatCount(summary.cacheWriteRequests)} · ${formatCount(
                summary.cacheWriteTokens
              )} tokens`}
              icon={<Database className='h-4 w-4' />}
            />
            <SummaryCard
              label={t('Cache read')}
              value={formatPercent(
                requestRatio(summary.cacheReadRequests, summary.totalRequests)
              )}
              hint={`${formatCount(summary.cacheReadRequests)} · ${formatCount(
                summary.cacheReadTokens
              )} tokens`}
              icon={<Database className='h-4 w-4' />}
            />
            <SummaryCard
              label={t('Input / output tokens')}
              value={`${formatCount(summary.inputTokens)} / ${formatCount(
                summary.outputTokens
              )}`}
              hint={t('Non-cached input and model output')}
              icon={<WalletCards className='h-4 w-4' />}
            />
          </div>

          <div className='min-h-0 flex-1 overflow-auto'>
            <StaticDataTable
              tableClassName='min-w-max'
              data={rows}
              getRowKey={usageRowKey}
              emptyContent={t('No usage data for this range')}
              emptyClassName='text-muted-foreground py-8'
              columns={[
                { id: 'date', header: t('Date'), cell: (row) => row.date },
                ...(canAdjustUsage
                  ? [
                      {
                        id: 'user',
                        header: t('User'),
                        cell: (row: ModelUsageRow) => (
                          <div className='flex flex-col leading-tight'>
                            <span>{row.username || '-'}</span>
                            <span className='text-muted-foreground text-xs'>
                              ID {row.user_id ?? '-'}
                            </span>
                          </div>
                        ),
                      },
                    ]
                  : []),
                {
                  id: 'token',
                  header: t('Key name'),
                  cellClassName: 'font-medium',
                  cell: (row) => (
                    <div className='flex flex-col leading-tight'>
                      <span>{row.token_name || '-'}</span>
                      <span className='text-muted-foreground text-xs'>
                        {t('Key ID')} {row.token_id ?? '-'}
                      </span>
                    </div>
                  ),
                },
                {
                  id: 'model',
                  header: t('Model name'),
                  cell: (row) => row.model_name || '-',
                },
                {
                  id: 'total-requests',
                  header: t('Total requests'),
                  cell: (row) => formatCount(row.total_requests),
                },
                {
                  id: 'cache-write-requests',
                  header: t('Cache write requests'),
                  cell: (row) => (
                    <div className='flex flex-col leading-tight'>
                      <span>{formatCount(row.cache_write_requests)}</span>
                      <span className='text-muted-foreground text-xs'>
                        5m {formatCount(row.cache_write_5m_requests)} · 1h{' '}
                        {formatCount(row.cache_write_1h_requests)}
                      </span>
                    </div>
                  ),
                },
                {
                  id: 'write-ratio',
                  header: t('Write ratio'),
                  cell: (row) =>
                    formatPercent(
                      requestRatio(row.cache_write_requests, row.total_requests)
                    ),
                },
                {
                  id: 'cache-read-requests',
                  header: t('Cache read requests'),
                  cell: (row) => formatCount(row.cache_read_requests),
                },
                {
                  id: 'read-ratio',
                  header: t('Read ratio'),
                  cell: (row) =>
                    formatPercent(
                      requestRatio(row.cache_read_requests, row.total_requests)
                    ),
                },
                {
                  id: 'cache-write-tokens',
                  header: t('Cache write tokens'),
                  cell: (row) => formatCount(row.cache_write_tokens),
                },
                {
                  id: 'cache-read-tokens',
                  header: t('Cache read tokens'),
                  cell: (row) => formatCount(row.cache_read_tokens),
                },
                {
                  id: 'input-tokens',
                  header: t('Input tokens'),
                  cell: (row) => formatCount(row.input_tokens),
                },
                {
                  id: 'output-tokens',
                  header: t('Output tokens'),
                  cell: (row) => formatCount(row.output_tokens),
                },
                {
                  id: 'latency',
                  header: t('Average latency'),
                  cell: (row) => (
                    <div className='flex flex-col leading-tight'>
                      <span>
                        {t('First token')} {formatMs(row.avg_first_token_ms)}
                      </span>
                      <span className='text-muted-foreground text-xs'>
                        {t('Request')} {formatMs(row.avg_use_time_ms)}
                      </span>
                    </div>
                  ),
                },
                {
                  id: 'cost',
                  header: t('Consumption (USD)'),
                  cellClassName: 'font-medium',
                  cell: (row) => formatUsd(row.cost_usd),
                },
                ...(canAdjustUsage
                  ? [
                      {
                        id: 'actions',
                        header: t('Actions'),
                        cell: (row: ModelUsageRow) => (
                          <Button
                            size='sm'
                            variant='outline'
                            onClick={() => setAdjustmentRow(row)}
                          >
                            <Pencil className='h-4 w-4' />
                            {t('Correct')}
                          </Button>
                        ),
                      },
                    ]
                  : []),
              ]}
            />
          </div>
          {canAdjustUsage && (
            <UsageAdjustmentDialog
              open={adjustmentRow !== null}
              row={adjustmentRow}
              onOpenChange={(open) => {
                if (!open) setAdjustmentRow(null)
              }}
              onSaved={() => void usageQuery.refetch()}
            />
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

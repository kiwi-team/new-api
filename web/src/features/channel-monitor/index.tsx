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
import { Activity, Gauge, RefreshCw, TriangleAlert } from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { getChannelMonitor } from './api'
import { MonitorDetailSheet } from './components/monitor-detail-sheet'
import { MonitorErrorList } from './components/monitor-error-list'
import { MonitorStatusBadge } from './components/monitor-status-badge'
import { TrendSparkline } from './components/trend-sparkline'
import {
  STATUS_META,
  TIME_RANGES,
  buildAxisLabels,
  buildErrorBreakdown,
  buildModelStats,
  buildSummary,
  formatCount,
  formatFirstSeconds,
  formatSeconds,
  getStatusLabel,
  getTimeRangeLabel,
  recordKey,
  recordVolume,
  timeRangeHours,
} from './lib'
import type { MonitorRecord, MonitorStatus, MonitorTimeRange } from './types'

/** How many models are pre-selected on first load. */
const DEFAULT_MODEL_FOCUS = 6
/** How many records the external overview highlights. */
const FOCUS_LIMIT = 6

const ALL_STATUSES = 'all'
const STATUS_FILTERS: (MonitorStatus | typeof ALL_STATUSES)[] = [
  ALL_STATUSES,
  'healthy',
  'degraded',
  'down',
  'unknown',
]

type ChannelMonitorPageProps = {
  /**
   * Internal triage view: shows channel names and raw upstream error text,
   * and lists every matching record instead of only the problem ones.
   */
  internalView?: boolean
}

function MetricCard(props: {
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

export function ChannelMonitorPage({
  internalView = false,
}: ChannelMonitorPageProps) {
  const { t } = useTranslation()
  const [scene, setScene] = useState<'overview' | 'errors'>('overview')
  const [timeRange, setTimeRange] = useState<MonitorTimeRange>('24h')
  const [statusFilter, setStatusFilter] = useState<string>(ALL_STATUSES)
  const [selectedModels, setSelectedModels] = useState<string[]>([])
  const [selected, setSelected] = useState<MonitorRecord | null>(null)
  const modelsInitialized = useRef(false)

  // The window is recomputed per range change so a refetch re-anchors to now.
  const window = useMemo(() => {
    const end = Math.floor(Date.now() / 1000)
    return { start: end - timeRangeHours(timeRange) * 3600, end }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [timeRange])

  const monitorQuery = useQuery({
    queryKey: ['channel-monitor', window.start, window.end],
    queryFn: () =>
      getChannelMonitor({
        start_timestamp: window.start,
        end_timestamp: window.end,
      }),
  })

  const records = useMemo(() => monitorQuery.data ?? [], [monitorQuery.data])
  const modelStats = useMemo(() => buildModelStats(records), [records])

  // On first load focus the busiest models so the list is not overwhelming.
  useEffect(() => {
    if (modelsInitialized.current || records.length === 0) return
    modelsInitialized.current = true
    setSelectedModels(
      buildModelStats(records)
        .slice(0, DEFAULT_MODEL_FOCUS)
        .map((item) => item.model)
    )
  }, [records])

  const filteredRecords = useMemo(() => {
    const models = new Set(selectedModels)
    return records
      .filter((record) => models.size === 0 || models.has(record.model))
      .filter(
        (record) =>
          statusFilter === ALL_STATUSES || record.status === statusFilter
      )
      .sort(
        (a, b) =>
          STATUS_META[a.status].order - STATUS_META[b.status].order ||
          recordVolume(b) - recordVolume(a)
      )
  }, [records, selectedModels, statusFilter])

  const summary = useMemo(
    () => buildSummary(filteredRecords),
    [filteredRecords]
  )
  const errorBreakdown = useMemo(
    () => buildErrorBreakdown(filteredRecords),
    [filteredRecords]
  )
  const axisLabels = useMemo(() => buildAxisLabels(window), [window])

  // The external view leads with what is broken; internal shows everything.
  const overviewRecords = useMemo(() => {
    if (internalView) return filteredRecords
    const issues = filteredRecords.filter(
      (record) => record.status !== 'healthy' || record.errors > 0
    )
    return (issues.length > 0 ? issues : filteredRecords).slice(0, FOCUS_LIMIT)
  }, [filteredRecords, internalView])

  const modelOptions = modelStats.map((item) => ({
    value: item.model,
    label: `${item.model} · ${formatCount(item.total)}`,
  }))
  const statusItems = STATUS_FILTERS.map((status) => ({
    value: status,
    label:
      status === ALL_STATUSES
        ? t('All statuses')
        : getStatusLabel(status as MonitorStatus, t),
  }))
  const rangeItems = TIME_RANGES.map((range) => ({
    value: range,
    label: getTimeRangeLabel(range, t),
  }))

  return (
    <>
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>
        {internalView
          ? t('Internal Channel Monitor')
          : t('Model Channel Monitor')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          size='sm'
          onClick={() => void monitorQuery.refetch()}
          disabled={monitorQuery.isFetching}
        >
          <RefreshCw className='h-4 w-4' />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-4'>
          <p className='text-muted-foreground text-sm'>
            {internalView
              ? t(
                  'Triage view: channel names, detailed errors and per-model availability trends.'
                )
              : t(
                  'Availability view: health, latency and success rate aggregated by key, channel and model.'
                )}
          </p>

          <div className='flex flex-wrap items-end gap-3'>
            <Tabs
              value={scene}
              onValueChange={(value) =>
                setScene(value as 'overview' | 'errors')
              }
            >
              <TabsList>
                <TabsTrigger value='overview'>{t('Health overview')}</TabsTrigger>
                <TabsTrigger value='errors'>{t('Failing channels')}</TabsTrigger>
              </TabsList>
            </Tabs>
            <div className='grid gap-1.5'>
              <Label>{t('Time range')}</Label>
              <Select
                items={rangeItems}
                value={timeRange}
                onValueChange={(value) =>
                  setTimeRange((value as MonitorTimeRange) ?? '24h')
                }
              >
                <SelectTrigger className='w-40'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {rangeItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className='grid gap-1.5'>
              <Label>{t('Models')}</Label>
              <MultiSelect
                className='w-72'
                options={modelOptions}
                selected={selectedModels}
                onChange={setSelectedModels}
                placeholder={t('All models')}
                maxVisibleChips={2}
              />
            </div>
            <div className='grid gap-1.5'>
              <Label>{t('Status')}</Label>
              <Select
                items={statusItems}
                value={statusFilter}
                onValueChange={(value) => setStatusFilter(value ?? ALL_STATUSES)}
              >
                <SelectTrigger className='w-36'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {statusItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className='grid grid-cols-2 gap-3 lg:grid-cols-4'>
            <MetricCard
              label={t('Success rate')}
              value={`${summary.successRate.toFixed(2)}%`}
              hint={t('{{total}} requests, {{errors}} errors', {
                total: formatCount(summary.totalRequests),
                errors: formatCount(summary.totalErrors),
              })}
              icon={<Activity className='h-4 w-4' />}
            />
            <MetricCard
              label={t('Average total time')}
              value={formatSeconds(summary.avgUse)}
              hint={t('Across records with samples')}
              icon={<Gauge className='h-4 w-4' />}
            />
            <MetricCard
              label={t('Average first token')}
              value={formatFirstSeconds(summary.avgFirst, t)}
              hint={t('Streaming requests only')}
              icon={<Gauge className='h-4 w-4' />}
            />
            <MetricCard
              label={t('Unhealthy')}
              value={`${summary.down} / ${summary.degraded}`}
              hint={t('Down / degraded combinations')}
              icon={<TriangleAlert className='h-4 w-4' />}
            />
          </div>

          <div className='min-h-0 flex-1 overflow-auto'>
            {scene === 'overview' ? (
              <div className='grid gap-3 lg:grid-cols-2'>
                {overviewRecords.length === 0 && (
                  <p className='text-muted-foreground py-8 text-sm'>
                    {t('No channel data for this range')}
                  </p>
                )}
                {overviewRecords.map((record) => (
                  <button
                    key={recordKey(record)}
                    type='button'
                    className='hover:bg-muted/40 space-y-3 rounded-lg border p-3 text-left transition-colors'
                    onClick={() => setSelected(record)}
                  >
                    <div className='flex items-start justify-between gap-2'>
                      <div className='min-w-0'>
                        <div className='truncate font-medium'>
                          {record.model}
                        </div>
                        <div className='text-muted-foreground truncate text-xs'>
                          {internalView
                            ? `${record.channelName} · ${record.keyName}`
                            : `${t('Key ID')} ${record.keyId} · ${record.keyHint}`}
                        </div>
                      </div>
                      <MonitorStatusBadge status={record.status} />
                    </div>
                    <div className='text-muted-foreground flex flex-wrap gap-3 text-xs'>
                      <span>
                        {t('Requests')} {formatCount(record.requests)}
                      </span>
                      <span>
                        {t('Errors')} {formatCount(record.errors)}
                      </span>
                      <span>
                        {t('Total time')} {formatSeconds(record.useMs)}
                      </span>
                      <span>
                        {t('First token')}{' '}
                        {formatFirstSeconds(record.firstMs, t)}
                      </span>
                    </div>
                    <TrendSparkline
                      values={record.trend}
                      errorMarks={record.errorMarks}
                      isDown={record.status === 'down'}
                      axisLabels={axisLabels}
                    />
                    <MonitorErrorList
                      record={record}
                      showSensitive={internalView}
                    />
                  </button>
                ))}
              </div>
            ) : (
              <div className='space-y-2'>
                {errorBreakdown.length === 0 && (
                  <p className='text-muted-foreground py-8 text-sm'>
                    {t('No aggregated errors in this range.')}
                  </p>
                )}
                {errorBreakdown.map((error, index) => (
                  <button
                    // eslint-disable-next-line react/no-array-index-key
                    key={`${recordKey(error.record)}-${error.code}-${index}`}
                    type='button'
                    className='hover:bg-muted/40 flex w-full items-start justify-between gap-3 rounded-lg border p-3 text-left transition-colors'
                    onClick={() => setSelected(error.record)}
                  >
                    <div className='min-w-0 space-y-0.5'>
                      <div className='truncate text-sm font-medium'>
                        {error.record.model}
                      </div>
                      <div className='text-muted-foreground truncate text-xs'>
                        {internalView
                          ? `${error.record.channelName} · ${error.record.keyName}`
                          : `${t('Key ID')} ${error.record.keyId} · ${error.record.keyHint}`}
                      </div>
                      <MonitorErrorList
                        record={{ ...error.record, errorsTop: [error] }}
                        limit={1}
                        showSensitive={internalView}
                      />
                    </div>
                    <MonitorStatusBadge status={error.record.status} />
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>

      <MonitorDetailSheet
        record={selected}
        onOpenChange={(open) => !open && setSelected(null)}
        showSensitive={internalView}
        axisLabels={axisLabels}
      />
    </>
  )
}

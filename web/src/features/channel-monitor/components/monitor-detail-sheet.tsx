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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import {
  formatCount,
  formatFirstSeconds,
  formatSeconds,
  formatTps,
} from '../lib'
import type { MonitorRecord } from '../types'
import { MonitorErrorList } from './monitor-error-list'
import { MonitorStatusBadge } from './monitor-status-badge'
import { TrendSparkline } from './trend-sparkline'

type MonitorDetailSheetProps = {
  record: MonitorRecord | null
  onOpenChange: (open: boolean) => void
  showSensitive: boolean
  axisLabels: string[]
}

type TrendTab = 'requests' | 'first' | 'use'

export function MonitorDetailSheet({
  record,
  onOpenChange,
  showSensitive,
  axisLabels,
}: MonitorDetailSheetProps) {
  const { t } = useTranslation()
  const [trendTab, setTrendTab] = useState<TrendTab>('requests')

  if (!record) return null

  const seriesByTab: Record<TrendTab, number[]> = {
    requests: record.trend,
    first: record.trendFirstMs ?? [],
    use: record.trendUseMs ?? [],
  }
  const series = seriesByTab[trendTab]

  const metrics = [
    { label: t('Requests'), value: formatCount(record.requests) },
    { label: t('Errors'), value: formatCount(record.errors) },
    { label: t('Average total time'), value: formatSeconds(record.useMs) },
    {
      label: t('Average first token'),
      value: formatFirstSeconds(record.firstMs, t),
    },
    { label: t('P95 total time'), value: formatSeconds(record.p95UseMs) },
    { label: t('Throughput'), value: formatTps(record.tps) },
  ]

  return (
    <Sheet open onOpenChange={onOpenChange}>
      <SheetContent className='w-full sm:max-w-xl'>
        <SheetHeader>
          <SheetTitle className='flex items-center gap-2'>
            {record.model}
            <MonitorStatusBadge status={record.status} />
          </SheetTitle>
          <SheetDescription>
            {showSensitive
              ? `${record.channelName} · ${t('Key ID')} ${record.keyId} · ${record.keyName}`
              : `${t('Key ID')} ${record.keyId} · ${record.keyHint}`}
          </SheetDescription>
        </SheetHeader>

        <div className='space-y-5 overflow-y-auto px-4 pb-4'>
          <div className='grid grid-cols-2 gap-3'>
            {metrics.map((metric) => (
              <div key={metric.label} className='rounded-lg border p-3'>
                <div className='text-muted-foreground text-xs'>
                  {metric.label}
                </div>
                <div className='mt-0.5 font-medium'>{metric.value}</div>
              </div>
            ))}
          </div>

          <div className='space-y-2'>
            <Tabs
              value={trendTab}
              onValueChange={(value) => setTrendTab(value as TrendTab)}
            >
              <TabsList>
                <TabsTrigger value='requests'>{t('Requests')}</TabsTrigger>
                <TabsTrigger value='first'>{t('First token')}</TabsTrigger>
                <TabsTrigger value='use'>{t('Total time')}</TabsTrigger>
              </TabsList>
            </Tabs>
            <TrendSparkline
              values={series}
              errorMarks={record.errorMarks}
              isDown={record.status === 'down'}
              axisLabels={axisLabels}
            />
          </div>

          <div className='space-y-2'>
            <h3 className='text-sm font-medium'>{t('Errors')}</h3>
            <MonitorErrorList
              record={record}
              limit={10}
              showSensitive={showSensitive}
            />
          </div>
        </div>
      </SheetContent>
    </Sheet>
  )
}

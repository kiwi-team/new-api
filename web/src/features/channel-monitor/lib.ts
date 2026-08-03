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
import type { TFunction } from 'i18next'

import type {
  MonitorModelStat,
  MonitorRecord,
  MonitorStatus,
  MonitorSummary,
  MonitorTimeRange,
} from './types'

/**
 * Sort weight and tone per status. Worse statuses sort first so the list
 * leads with what needs attention.
 */
export const STATUS_META: Record<
  MonitorStatus,
  { order: number; tone: 'success' | 'warning' | 'danger' | 'neutral' }
> = {
  down: { order: 1, tone: 'danger' },
  degraded: { order: 2, tone: 'warning' },
  healthy: { order: 3, tone: 'success' },
  unknown: { order: 4, tone: 'neutral' },
}

export function getStatusLabel(status: MonitorStatus, t: TFunction): string {
  switch (status) {
    case 'healthy':
      return t('Healthy')
    case 'degraded':
      return t('Degraded')
    case 'down':
      return t('Down')
    default:
      return t('Unknown')
  }
}

export const TIME_RANGES: MonitorTimeRange[] = ['1h', '6h', '24h']

export function timeRangeHours(range: MonitorTimeRange): number {
  if (range === '1h') return 1
  if (range === '6h') return 6
  return 24
}

export function getTimeRangeLabel(
  range: MonitorTimeRange,
  t: TFunction
): string {
  switch (range) {
    case '1h':
      return t('Last 1 hour')
    case '6h':
      return t('Last 6 hours')
    default:
      return t('Last 24 hours')
  }
}

/** Totals are shown in seconds; the API reports milliseconds. */
export function formatSeconds(value: number | null | undefined): string {
  if (value === null || value === undefined) return '-'
  return `${Number(value / 1000).toLocaleString(undefined, {
    maximumFractionDigits: 1,
  })} s`
}

/**
 * First-token latency only exists for streaming requests, so a missing value
 * means "not instrumented" rather than zero.
 */
export function formatFirstSeconds(
  value: number | null | undefined,
  t: TFunction
): string {
  if (value === null || value === undefined) return t('No samples')
  return `${Number(value / 1000).toLocaleString(undefined, {
    maximumFractionDigits: 2,
  })} s`
}

export function formatCount(value: number | null | undefined): string {
  return Number(value ?? 0).toLocaleString()
}

export function formatTps(value: number | null | undefined): string {
  if (value === null || value === undefined) return '-'
  return `${Number(value).toFixed(1)} tok/s`
}

/** Latency tone thresholds, in ms. */
export function latencyTone(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return ''
  if (ms > 120_000) return 'danger'
  if (ms > 60_000) return 'warning'
  return 'success'
}

export function firstTokenTone(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return ''
  if (ms > 5000) return 'danger'
  if (ms > 2000) return 'warning'
  return 'success'
}

export function recordVolume(record: MonitorRecord): number {
  return record.requests + record.errors
}

export function recordKey(record: MonitorRecord): string {
  return `${record.channelId}_${record.keyId}_${record.model}`
}

/** Same thresholds the backend uses per record, applied to a rollup. */
export function aggregateStatus(
  successRate: number,
  p95UseMs: number
): MonitorStatus {
  if (successRate < 90) return 'down'
  if (successRate < 98 || p95UseMs > 120_000) return 'degraded'
  return 'healthy'
}

export function buildModelStats(records: MonitorRecord[]): MonitorModelStat[] {
  const byModel = new Map<string, Omit<MonitorModelStat, 'total' | 'successRate' | 'status'>>()
  for (const record of records) {
    const current = byModel.get(record.model) ?? {
      model: record.model,
      requests: 0,
      errors: 0,
      p95UseMs: 0,
      records: 0,
    }
    current.requests += record.requests
    current.errors += record.errors
    current.p95UseMs = Math.max(current.p95UseMs, record.p95UseMs || 0)
    current.records += 1
    byModel.set(record.model, current)
  }
  return [...byModel.values()]
    .map((item) => {
      const total = item.requests + item.errors
      const successRate = total ? (item.requests / total) * 100 : 0
      return {
        ...item,
        total,
        successRate,
        status: aggregateStatus(successRate, item.p95UseMs),
      }
    })
    .sort((a, b) => b.total - a.total)
}

export function buildSummary(records: MonitorRecord[]): MonitorSummary {
  const totalRequests = records.reduce(
    (sum, record) => sum + record.requests + record.errors,
    0
  )
  const totalSuccess = records.reduce((sum, record) => sum + record.requests, 0)
  const totalErrors = records.reduce((sum, record) => sum + record.errors, 0)
  const useSamples = records.filter((record) => record.useMs !== null)
  // Records without a streaming sample must not drag the average toward zero.
  const firstSamples = records.filter(
    (record) => record.firstMs !== null && record.firstMs !== undefined
  )
  return {
    totalRequests,
    totalErrors,
    successRate: totalRequests ? (totalSuccess / totalRequests) * 100 : 0,
    avgUse: useSamples.length
      ? useSamples.reduce((sum, record) => sum + (record.useMs ?? 0), 0) /
        useSamples.length
      : 0,
    avgFirst: firstSamples.length
      ? firstSamples.reduce((sum, record) => sum + (record.firstMs ?? 0), 0) /
        firstSamples.length
      : null,
    down: records.filter((record) => record.status === 'down').length,
    degraded: records.filter((record) => record.status === 'degraded').length,
  }
}

/** Flatten every record's top errors into one list, worst first. */
export function buildErrorBreakdown(records: MonitorRecord[]) {
  return records
    .flatMap((record) =>
      record.errorsTop.map((error, index) => ({
        ...error,
        index,
        record,
      }))
    )
    .sort((a, b) => b.count - a.count)
}

/**
 * The external view hides upstream error text, which can carry provider
 * account details; the internal view is for triage and shows everything.
 */
export function errorPreview(
  error: { code?: string; text: string },
  showSensitive: boolean,
  t: TFunction
): string {
  if (showSensitive) {
    return error.code ? `${error.code}: ${error.text}` : error.text
  }
  return error.code ? `${t('Error code')} ${error.code}` : t('Uncategorized error')
}

/** Five evenly spaced HH:MM ticks across the loaded window. */
export function buildAxisLabels(
  window: { start: number; end: number } | null
): string[] {
  if (!window) return []
  const span = window.end - window.start
  return [0, 0.25, 0.5, 0.75, 1].map((fraction) => {
    const date = new Date((window.start + span * fraction) * 1000)
    return `${String(date.getHours()).padStart(2, '0')}:${String(
      date.getMinutes()
    ).padStart(2, '0')}`
  })
}

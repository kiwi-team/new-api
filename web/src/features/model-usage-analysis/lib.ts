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

import type { ModelUsageRow, ModelUsageSummary } from './types'

export function formatCount(value: number | undefined): string {
  return Number(value || 0).toLocaleString()
}

export function formatUsd(value: number | undefined): string {
  return `$${Number(value || 0).toLocaleString(undefined, {
    maximumFractionDigits: 2,
    minimumFractionDigits: 2,
  })}`
}

export function formatPercent(value: number): string {
  return `${Number(value || 0).toFixed(2)}%`
}

export function formatMs(value: number | undefined): string {
  const parsed = Number(value || 0)
  return parsed > 0 ? `${parsed.toLocaleString()} ms` : '-'
}

/** Share of requests that hit a given cache path, guarding divide-by-zero. */
export function requestRatio(
  part: number | undefined,
  total: number | undefined
): number {
  return total ? ((part || 0) / total) * 100 : 0
}

export function buildUsageSummary(rows: ModelUsageRow[]): ModelUsageSummary {
  return rows.reduce<ModelUsageSummary>(
    (summary, row) => ({
      costUsd: summary.costUsd + (row.cost_usd || 0),
      totalRequests: summary.totalRequests + (row.total_requests || 0),
      cacheWriteRequests:
        summary.cacheWriteRequests + (row.cache_write_requests || 0),
      cacheReadRequests:
        summary.cacheReadRequests + (row.cache_read_requests || 0),
      cacheWriteTokens:
        summary.cacheWriteTokens + (row.cache_write_tokens || 0),
      cacheReadTokens: summary.cacheReadTokens + (row.cache_read_tokens || 0),
      inputTokens: summary.inputTokens + (row.input_tokens || 0),
      outputTokens: summary.outputTokens + (row.output_tokens || 0),
    }),
    {
      costUsd: 0,
      totalRequests: 0,
      cacheWriteRequests: 0,
      cacheReadRequests: 0,
      cacheWriteTokens: 0,
      cacheReadTokens: 0,
      inputTokens: 0,
      outputTokens: 0,
    }
  )
}

export function usageRowKey(row: ModelUsageRow): string {
  return [row.date, row.token_id ?? '', row.model_name ?? ''].join('_')
}

function csvCell(value: unknown): string {
  const text = value == null ? '' : String(value)
  return /["\n,]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text
}

function pad2(value: number): string {
  return String(value).padStart(2, '0')
}

function formatChinaDate(date: Date): string {
  return `${date.getUTCFullYear()}-${pad2(date.getUTCMonth() + 1)}-${pad2(
    date.getUTCDate()
  )}`
}

export function getDefaultUsageDateRange(now = new Date()): {
  start: string
  end: string
} {
  const chinaNow = new Date(now.getTime() + 8 * 3600 * 1000)
  const end = formatChinaDate(chinaNow)
  const startValue = new Date(chinaNow)
  startValue.setUTCDate(startValue.getUTCDate() - 6)
  return { start: formatChinaDate(startValue), end }
}

export function usageDateRangeTimestamps(
  start: string,
  end: string
): {
  start_timestamp: number
  end_timestamp: number
} {
  return {
    start_timestamp: Math.floor(Date.parse(`${start}T00:00:00+08:00`) / 1000),
    end_timestamp: Math.floor(Date.parse(`${end}T23:59:59+08:00`) / 1000),
  }
}

export function usageHourTimestamp(date: string, hour: number): number {
  return Math.floor(Date.parse(`${date}T${pad2(hour)}:00:00+08:00`) / 1000)
}

/**
 * Build the CSV client-side — this report has no export endpoint, and the
 * rows are already fully aggregated by the time they reach the browser.
 */
export function exportUsageRowsCsv(
  rows: ModelUsageRow[],
  range: { start: string; end: string },
  t: TFunction
): void {
  const columns: { header: string; get: (row: ModelUsageRow) => unknown }[] = [
    { header: t('Date'), get: (row) => row.date },
    { header: t('Key name'), get: (row) => row.token_name },
    { header: t('Key ID'), get: (row) => row.token_id },
    { header: t('Model name'), get: (row) => row.model_name },
    { header: t('Total requests'), get: (row) => row.total_requests || 0 },
    {
      header: t('Cache write requests'),
      get: (row) => row.cache_write_requests || 0,
    },
    {
      header: t('Cache write 5m requests'),
      get: (row) => row.cache_write_5m_requests || 0,
    },
    {
      header: t('Cache write 1h requests'),
      get: (row) => row.cache_write_1h_requests || 0,
    },
    {
      header: t('Write ratio (%)'),
      get: (row) =>
        requestRatio(row.cache_write_requests, row.total_requests).toFixed(2),
    },
    {
      header: t('Cache read requests'),
      get: (row) => row.cache_read_requests || 0,
    },
    {
      header: t('Read ratio (%)'),
      get: (row) =>
        requestRatio(row.cache_read_requests, row.total_requests).toFixed(2),
    },
    {
      header: t('Cache write tokens'),
      get: (row) => row.cache_write_tokens || 0,
    },
    {
      header: t('Cache read tokens'),
      get: (row) => row.cache_read_tokens || 0,
    },
    { header: t('Input tokens'), get: (row) => row.input_tokens || 0 },
    { header: t('Output tokens'), get: (row) => row.output_tokens || 0 },
    {
      header: t('Average first token (ms)'),
      get: (row) => row.avg_first_token_ms || 0,
    },
    {
      header: t('Average request time (ms)'),
      get: (row) => row.avg_use_time_ms || 0,
    },
    {
      header: t('Consumption (USD)'),
      get: (row) => Number(row.cost_usd || 0).toFixed(6),
    },
  ]

  const lines = [columns.map((column) => csvCell(column.header)).join(',')]
  for (const row of rows) {
    lines.push(columns.map((column) => csvCell(column.get(row))).join(','))
  }
  // Excel needs the BOM to read UTF-8 correctly.
  const csv = `﻿${lines.join('\r\n')}`
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
  const url = window.URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `model-usage-${range.start.replaceAll('-', '')}-${range.end.replaceAll('-', '')}.csv`
  link.click()
  window.URL.revokeObjectURL(url)
}

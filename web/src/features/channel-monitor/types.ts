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
export type MonitorStatus = 'healthy' | 'degraded' | 'down' | 'unknown'

/** One aggregated error bucket inside a monitor record. */
export type MonitorError = {
  code?: string
  text: string
  count: number
  /** Human-readable timestamp of the most recent occurrence */
  last: string
}

/** One key × channel × model combination over the selected window. */
export type MonitorRecord = {
  channelId: number
  channelName: string
  channelTypeId: number
  keyId: number
  keyName: string
  keyHint: string
  model: string
  status: MonitorStatus
  requests: number
  errors: number
  /** Average total latency in ms; null when there is no sample */
  useMs: number | null
  /** Average first-token latency in ms; only streaming requests report it */
  firstMs: number | null
  p95UseMs: number | null
  tps: number | null
  /** Per-bucket request counts used by the sparkline */
  trend: number[]
  trendFirstMs?: number[]
  trendUseMs?: number[]
  /** Bucket indexes that saw errors */
  errorMarks: number[]
  errorsTop: MonitorError[]
}

/** Per-model rollup shown in the model picker and the health list. */
export type MonitorModelStat = {
  model: string
  requests: number
  errors: number
  p95UseMs: number
  records: number
  total: number
  successRate: number
  status: MonitorStatus
}

export type MonitorSummary = {
  totalRequests: number
  totalErrors: number
  successRate: number
  avgUse: number
  avgFirst: number | null
  down: number
  degraded: number
}

export type MonitorTimeRange = '1h' | '6h' | '24h'

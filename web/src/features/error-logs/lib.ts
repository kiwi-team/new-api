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
import type { ErrorLog } from './types'

/** Nested request ids the relay stores inside the `extra` JSON blob. */
export type ErrorLogExtra = {
  mt_session_id?: string
  trace_id?: string
  traj_id?: string
  [key: string]: unknown
}

export function parseErrorLogExtra(extra: string | null | undefined): ErrorLogExtra {
  if (!extra) return {}
  try {
    const parsed = JSON.parse(extra)
    return parsed && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}

/** Latency is stored in ms; anything under a second reads better as ms. */
export function formatUseTime(ms: number | undefined): string {
  const value = Number(ms || 0)
  if (value <= 0) return '-'
  if (value < 1000) return `${value} ms`
  return `${(value / 1000).toFixed(2)} s`
}

/** HTTP status tone: 4xx is caller error, 5xx is upstream/relay failure. */
export function statusTone(
  status: number | undefined
): 'danger' | 'warning' | 'neutral' {
  const code = Number(status || 0)
  if (code >= 500) return 'danger'
  if (code >= 400) return 'warning'
  return 'neutral'
}

export function errorLogRowKey(log: ErrorLog): number {
  return log.id
}

/** The export endpoint refuses ranges wider than this. */
export const EXPORT_MAX_RANGE_HOURS = 24

export function isRangeExportable(
  start: Date | undefined,
  end: Date | undefined
): boolean {
  if (!start || !end) return false
  const hours = (end.getTime() - start.getTime()) / 3_600_000
  return hours > 0 && hours <= EXPORT_MAX_RANGE_HOURS
}

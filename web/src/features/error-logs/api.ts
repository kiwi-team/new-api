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
import { api } from '@/lib/api'

import type { ApiResponse, ErrorLogFilters, ErrorLogListResponse } from './types'

function toParams(filters: ErrorLogFilters) {
  const params: Record<string, string | number> = {}
  for (const [key, value] of Object.entries(filters)) {
    if (value === undefined || value === '' || value === 0) continue
    params[key] = value as string | number
  }
  return params
}

export async function getErrorLogs(
  params: ErrorLogFilters & { p: number; page_size: number }
): Promise<ErrorLogListResponse> {
  const { p, page_size, ...filters } = params
  const res = await api.get('/api/log/error-logs', {
    params: { p, page_size, ...toParams(filters) },
  })
  return res.data
}

/**
 * Request bodies are large, so they are fetched per row on demand rather
 * than inlined in the list response.
 */
export async function getErrorLogBody(
  id: number
): Promise<ApiResponse<string>> {
  const res = await api.get(`/api/log/error-logs/${id}/body`)
  return res.data
}

/** Root only: request headers can carry upstream credentials. */
export async function getErrorLogHeader(
  id: number
): Promise<ApiResponse<string>> {
  const res = await api.get(`/api/log/error-logs/${id}/header`)
  return res.data
}

/** Root only; the backend caps the range at 24 hours. */
export async function exportErrorLogsCsv(
  filters: ErrorLogFilters
): Promise<Blob> {
  const res = await api.get('/api/log/error-logs/export', {
    params: toParams(filters),
    responseType: 'blob',
  })
  return res.data as Blob
}

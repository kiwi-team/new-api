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

import type { QuotaStatisticsQuery, QuotaStatisticsRow, TokenOption } from './types'

function toParams(query: QuotaStatisticsQuery) {
  return {
    start_timestamp: query.start_timestamp,
    end_timestamp: query.end_timestamp,
    model_name: query.model_name ?? '',
    client_user_id: query.client_user_id ?? '',
    client_scenairos: query.client_scenairos ?? '',
    expand_models: query.expand_models,
    expand_dates: query.expand_dates,
    expand_tokens: query.expand_tokens,
    ...(query.user_id ? { user_id: query.user_id } : {}),
    ...(query.project_name ? { project_name: query.project_name } : {}),
    ...(query.token_ids ? { token_ids: query.token_ids } : {}),
  }
}

export async function getQuotaStatistics(
  query: QuotaStatisticsQuery
): Promise<QuotaStatisticsRow[]> {
  const res = await api.get('/api/data/statistics', { params: toParams(query) })
  if (!res.data?.success) {
    throw new Error(res.data?.message || 'Failed to load')
  }
  return Array.isArray(res.data.data) ? res.data.data : []
}

export async function exportQuotaStatisticsCsv(
  query: QuotaStatisticsQuery
): Promise<Blob> {
  const res = await api.get('/api/data/statistics/export', {
    params: toParams(query),
    responseType: 'blob',
  })
  return res.data as Blob
}

/** Keys available as a filter; scoped by the backend to the caller. */
export async function getTokenOptions(): Promise<TokenOption[]> {
  const res = await api.get('/api/data/token-list')
  if (!res.data?.success) return []
  return Array.isArray(res.data.data) ? res.data.data : []
}

export async function getProjectNames(): Promise<string[]> {
  const res = await api.get('/api/data/project-names')
  if (!res.data?.success) return []
  return Array.isArray(res.data.data) ? res.data.data : []
}

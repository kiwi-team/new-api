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

import type {
  ApiResponse,
  ClientUserQuotaListResponse,
  ClientUserQuotaPayload,
  ProjectAllocation,
  ProjectBudgetSummary,
} from './types'

type ListParams = {
  p: number
  page_size: number
  keyword?: string
}

export async function getClientUserQuotas(
  params: ListParams
): Promise<ClientUserQuotaListResponse> {
  const { keyword, ...pageParams } = params
  const trimmed = keyword?.trim()
  const res = trimmed
    ? await api.get('/api/cliend_user_quota/search', {
        params: { ...pageParams, keyword: trimmed },
      })
    : await api.get('/api/cliend_user_quota/', { params: pageParams })
  return res.data
}

export async function createClientUserQuota(
  payload: ClientUserQuotaPayload
): Promise<ApiResponse> {
  const res = await api.post('/api/cliend_user_quota/', payload)
  return res.data
}

export async function updateClientUserQuota(
  payload: ClientUserQuotaPayload & { id: number }
): Promise<ApiResponse> {
  const res = await api.put('/api/cliend_user_quota/', payload)
  return res.data
}

export async function deleteClientUserQuota(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/cliend_user_quota/${id}`)
  return res.data
}

/**
 * Project budget rollups for the UIDs of the current page, fetched in one
 * request so the list does not fan out per row.
 */
export async function getBatchProjectBudget(
  clientUserIds: string[]
): Promise<Record<string, ProjectBudgetSummary>> {
  if (clientUserIds.length === 0) return {}
  const res = await api.get('/api/cliend_user_quota/batch-project-budget', {
    params: { uids: clientUserIds.join(',') },
  })
  if (!res.data?.success) return {}
  return (res.data.data ?? {}) as Record<string, ProjectBudgetSummary>
}

export async function getProjectAllocations(
  clientUserId: string
): Promise<ProjectAllocation[]> {
  const res = await api.get('/api/cliend_user_quota/project-allocations', {
    params: { client_user_id: clientUserId },
  })
  if (!res.data?.success) return []
  return Array.isArray(res.data.data) ? res.data.data : []
}

/** Server-rendered CSV of every UID budget the caller may see. */
export async function exportClientUserQuotasCsv(): Promise<Blob> {
  const res = await api.get('/api/cliend_user_quota/export', {
    responseType: 'blob',
  })
  return res.data as Blob
}

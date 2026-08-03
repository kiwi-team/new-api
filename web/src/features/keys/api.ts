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
  ApiKey,
  ApiResponse,
  GetApiKeysParams,
  GetApiKeysResponse,
  SearchApiKeysParams,
  ApiKeyFormData,
} from './types'

// ============================================================================
// API Key Management
// ============================================================================

// Get paginated API keys list
export async function getApiKeys(
  params: GetApiKeysParams = {}
): Promise<GetApiKeysResponse> {
  const { p = 1, size = 10, userId } = params
  const queryParams = new URLSearchParams({ p: String(p), size: String(size) })
  // Root only, ignored server-side otherwise: a specific id scopes the list to
  // that user, 0 returns every user's keys.
  if (userId != null) queryParams.set('user_id', String(userId))
  const res = await api.get(`/api/token/?${queryParams.toString()}`)
  return res.data
}

// Search API keys by keyword or token (with pagination)
export async function searchApiKeys(
  params: SearchApiKeysParams
): Promise<GetApiKeysResponse> {
  const { keyword = '', token = '', p, size, userId } = params
  const queryParams = new URLSearchParams()
  if (keyword) queryParams.set('keyword', keyword)
  if (token) queryParams.set('token', token)
  if (p != null) queryParams.set('p', String(p))
  if (size != null) queryParams.set('size', String(size))
  if (userId != null) queryParams.set('user_id', String(userId))
  const res = await api.get(`/api/token/search?${queryParams.toString()}`)
  return res.data
}

// Get single API key by ID
export async function getApiKey(id: number): Promise<ApiResponse<ApiKey>> {
  const res = await api.get(`/api/token/${id}`)
  return res.data
}

// Create a new API key
export async function createApiKey(
  data: ApiKeyFormData
): Promise<ApiResponse<ApiKey>> {
  const res = await api.post('/api/token/', data)
  return res.data
}

// Update an existing API key
export async function updateApiKey(
  data: ApiKeyFormData & { id: number }
): Promise<ApiResponse<ApiKey>> {
  const res = await api.put('/api/token/', data)
  return res.data
}

// Delete a single API key
export async function deleteApiKey(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/token/${id}/`)
  return res.data
}

// Batch delete multiple API keys
export async function batchDeleteApiKeys(
  ids: number[]
): Promise<ApiResponse<number>> {
  const res = await api.post('/api/token/batch', { ids })
  return res.data
}

// Update API key status (enable/disable)
export async function updateApiKeyStatus(
  id: number,
  status: number
): Promise<ApiResponse<ApiKey>> {
  const res = await api.put('/api/token/?status_only=true', { id, status })
  return res.data
}

// Fetch the real (unmasked) key for a token by ID
export async function fetchTokenKey(
  id: number
): Promise<{ success: boolean; message?: string; data?: { key: string } }> {
  const res = await api.post(`/api/token/${id}/key`)
  return res.data
}

// Batch fetch real (unmasked) keys for multiple tokens
export async function fetchTokenKeysBatch(ids: number[]): Promise<{
  success: boolean
  message?: string
  data?: { keys: Record<number, string> }
}> {
  const res = await api.post('/api/token/batch/keys', { ids })
  return res.data
}

/**
 * Assign a group to the selected keys.
 *
 * Non-root callers are scoped to their own keys server-side; root may set the
 * group on anyone's.
 */
export async function batchSetApiKeyGroup(
  ids: number[],
  group: string
): Promise<ApiResponse<number>> {
  const res = await api.post('/api/token/batch/group', { ids, group })
  return res.data
}

/**
 * Append models to every key in a group.
 *
 * Note this is group-scoped, not selection-scoped: the backend
 * (`BatchAppendTokenModelsByGroup`) matches on the group name, so the current
 * row selection is irrelevant here.
 */
export async function batchAppendApiKeyModels(
  group: string,
  models: string[]
): Promise<ApiResponse<number>> {
  const res = await api.post('/api/token/batch/models', { group, models })
  return res.data
}

export type UserOption = { id: number; username: string }

/**
 * Users available in the root-only key filter.
 *
 * `/api/user/` is admin-gated and paginated; one large page is enough for a
 * picker and avoids a per-keystroke lookup.
 */
export async function getUserOptions(): Promise<UserOption[]> {
  const res = await api.get('/api/user/', {
    params: { p: 1, page_size: 1000 },
  })
  if (!res.data?.success) return []
  const items = res.data.data?.items
  return Array.isArray(items)
    ? items.map((user: UserOption) => ({
        id: user.id,
        username: user.username,
      }))
    : []
}

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

import { buildQueryParams } from './lib/build-query-params'
import type {
  GetLogsParams,
  GetLogsResponse,
  GetLogStatsParams,
  GetLogStatsResponse,
  GetMidjourneyLogsParams,
  GetTaskLogsParams,
  UserInfo,
} from './types'

export type LogFilterOption = {
  value: string
  label: string
}

type FilterOptionApiResponse<T> = {
  success: boolean
  data?: T
}

type TokenOptionSource = {
  id?: number
  name?: string
}

type ChannelOptionSource = {
  id?: number
  name?: string
}

type UserOptionSource = {
  id?: number
  username?: string
}

type ClientUidOptionSource = {
  client_user_id?: string
  client_name?: string
}

type FilterOptionPage<T> = {
  items?: T[]
}

// ============================================================================
// Generic API Helpers
// ============================================================================

function buildApiPath(endpoint: string, isAdmin: boolean): string {
  return isAdmin ? endpoint : `${endpoint}/self`
}

async function fetchLogs<T>(
  endpoint: string,
  params: T,
  isAdmin: boolean
): Promise<GetLogsResponse> {
  const paramRecord = params as unknown as Record<string, unknown>
  const queryParams = buildQueryParams({
    p: paramRecord.p || 1,
    page_size: paramRecord.page_size || 20,
    ...params,
  })
  const path = buildApiPath(endpoint, isAdmin)
  const res = await api.get(`${path}?${queryParams}`)
  return res.data
}

async function fetchLogStats<T>(
  endpoint: string,
  params: T,
  isAdmin: boolean
): Promise<GetLogStatsResponse> {
  const queryParams = buildQueryParams(
    params as unknown as Record<string, unknown>
  )
  const path = buildApiPath(endpoint, isAdmin)
  const res = await api.get(`${path}/stat?${queryParams}`)
  return res.data
}

// ============================================================================
// Common Log APIs
// ============================================================================

export const getAllLogs = (params: GetLogsParams = {}) =>
  fetchLogs('/api/log', params, true)

export const getUserLogs = (
  params: Omit<GetLogsParams, 'username' | 'channel'> = {}
) => fetchLogs('/api/log', params, false)

export const getLogStats = (params: GetLogStatsParams = {}) =>
  fetchLogStats('/api/log', params, true)

export const getUserLogStats = (
  params: Omit<GetLogStatsParams, 'username' | 'channel'> = {}
) => fetchLogStats('/api/log', params, false)

export async function getLogTokenOptions(): Promise<LogFilterOption[]> {
  const res = await api.get<FilterOptionApiResponse<TokenOptionSource[]>>(
    '/api/data/token-list'
  )
  if (!res.data.success || !Array.isArray(res.data.data)) return []

  const names = new Set<string>()
  for (const token of res.data.data) {
    const name = token.name?.trim()
    if (name) names.add(name)
  }
  return [...names].map((name) => ({ value: name, label: name }))
}

export async function getLogTokenIdOptions(): Promise<LogFilterOption[]> {
  const res = await api.get<FilterOptionApiResponse<TokenOptionSource[]>>(
    '/api/data/token-list'
  )
  if (!res.data.success || !Array.isArray(res.data.data)) return []

  return res.data.data.flatMap((token) => {
    if (token.id == null) return []
    const value = String(token.id)
    const name = token.name?.trim()
    return [{ value, label: name ? `${name} (ID: ${value})` : `#${value}` }]
  })
}

export async function getLogChannelOptions(): Promise<LogFilterOption[]> {
  const res = await api.get<FilterOptionApiResponse<ChannelOptionSource[]>>(
    '/api/channel/channel-name-list'
  )
  if (!res.data.success || !Array.isArray(res.data.data)) return []

  return res.data.data.flatMap((channel) => {
    if (channel.id == null) return []
    const value = String(channel.id)
    const name = channel.name?.trim()
    return [{ value, label: name ? `${name} (ID: ${value})` : `#${value}` }]
  })
}

export async function getLogUsernameOptions(
  keyword: string
): Promise<LogFilterOption[]> {
  const trimmed = keyword.trim()
  const path = trimmed ? '/api/user/search' : '/api/user/'
  const res = await api.get<
    FilterOptionApiResponse<FilterOptionPage<UserOptionSource>>
  >(path, {
    params: {
      p: 1,
      page_size: 20,
      ...(trimmed ? { keyword: trimmed } : {}),
    },
  })
  const items = res.data.data?.items
  if (!res.data.success || !Array.isArray(items)) return []

  return items.flatMap((user) => {
    const username = user.username?.trim()
    if (!username) return []
    const idSuffix = user.id == null ? '' : ` (ID: ${user.id})`
    return [{ value: username, label: `${username}${idSuffix}` }]
  })
}

export async function getLogClientUidOptions(
  keyword: string
): Promise<LogFilterOption[]> {
  const trimmed = keyword.trim()
  const path = trimmed
    ? '/api/cliend_user_quota/search'
    : '/api/cliend_user_quota/'
  const res = await api.get<
    FilterOptionApiResponse<FilterOptionPage<ClientUidOptionSource>>
  >(path, {
    params: {
      p: 1,
      page_size: 50,
      ...(trimmed ? { keyword: trimmed } : {}),
    },
  })
  const items = res.data.data?.items
  if (!res.data.success || !Array.isArray(items)) return []

  return items.flatMap((item) => {
    const clientUserId = item.client_user_id?.trim()
    if (!clientUserId) return []
    const clientName = item.client_name?.trim()
    return [
      {
        value: clientUserId,
        label: clientName ? `${clientUserId} (${clientName})` : clientUserId,
      },
    ]
  })
}

export async function getUserInfo(
  userId: number
): Promise<{ success: boolean; message?: string; data?: UserInfo }> {
  const res = await api.get(`/api/user/${userId}`)
  return res.data
}

// ============================================================================
// MjProxy (Drawing) Logs API
// ============================================================================

export const getAllMidjourneyLogs = (params: GetMidjourneyLogsParams) =>
  fetchLogs('/api/mj', params, true)

export const getUserMidjourneyLogs = (params: GetMidjourneyLogsParams) =>
  fetchLogs('/api/mj', params, false)

// ============================================================================
// Task Logs API
// ============================================================================

export const getAllTaskLogs = (params: GetTaskLogsParams) =>
  fetchLogs('/api/task', params, true)

export const getUserTaskLogs = (params: GetTaskLogsParams) =>
  fetchLogs('/api/task', params, false)

// ============================================================================
// Raw payload API (root only)
// ============================================================================

/**
 * Request/response bodies and headers are fetched per log on demand: they are
 * large, and the backend keeps all three behind `RootAuth` because they can
 * contain prompts and upstream credentials.
 */
async function getLogPayload(
  id: number,
  kind: 'request' | 'response' | 'header'
): Promise<string> {
  const res = await api.get(`/api/log/${id}/${kind}`)
  if (!res.data?.success) {
    throw new Error(res.data?.message || 'Failed to load')
  }
  return String(res.data.data?.content ?? '')
}

export const getLogRequestBody = (id: number) => getLogPayload(id, 'request')
export const getLogResponseBody = (id: number) => getLogPayload(id, 'response')
export const getLogHeaders = (id: number) => getLogPayload(id, 'header')

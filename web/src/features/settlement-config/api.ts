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
import { getUsers, searchUsers } from '@/features/users/api'
import { api } from '@/lib/api'

import type {
  ApiResponse,
  SettlementConfig,
  SettlementConfigPayload,
  UserOption,
} from './types'

/** Settlement overrides of one customer. */
export async function getSettlementConfigs(
  userId: number
): Promise<SettlementConfig[]> {
  const res = await api.get('/api/settlement/config', {
    params: { user_id: userId },
  })
  if (!res.data?.success) return []
  return Array.isArray(res.data.data) ? res.data.data : []
}

/** Every customer's overrides, used by the Pricing Center's model rollup. */
export async function getAllSettlementConfigs(): Promise<SettlementConfig[]> {
  const res = await api.get('/api/settlement/config/all')
  if (!res.data?.success) return []
  return Array.isArray(res.data.data) ? res.data.data : []
}

export async function createSettlementConfig(
  payload: SettlementConfigPayload
): Promise<ApiResponse> {
  const res = await api.post('/api/settlement/config', payload)
  return res.data
}

export async function updateSettlementConfig(
  payload: SettlementConfigPayload & { id: number }
): Promise<ApiResponse> {
  const res = await api.put('/api/settlement/config', payload)
  return res.data
}

export async function deleteSettlementConfig(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/settlement/config/${id}`)
  return res.data
}

/** Import `{user_id, configs: [...]}` in one request. */
export async function batchImportSettlementConfigs(
  payload: unknown
): Promise<ApiResponse> {
  const res = await api.post('/api/settlement/config/batch', payload)
  return res.data
}

/** Remote user picker source: keyword search, or the first page when empty. */
export async function searchUserOptions(keyword: string): Promise<UserOption[]> {
  const trimmed = keyword.trim()
  const res = trimmed
    ? await searchUsers({ keyword: trimmed, p: 1, page_size: 20 })
    : await getUsers({ p: 1, page_size: 20 })
  if (!res.success) return []
  const data = res.data as unknown
  const items = (
    Array.isArray(data) ? data : ((data as { items?: unknown[] })?.items ?? [])
  ) as { id: number; username: string }[]
  return items.map((user) => ({
    value: user.id,
    label: `${user.username} (ID: ${user.id})`,
  }))
}

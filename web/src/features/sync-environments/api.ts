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
  SyncEnvironment,
  SyncEnvironmentPayload,
  SyncLog,
} from './types'

export async function getSyncEnvironments(): Promise<SyncEnvironment[]> {
  const res = await api.get('/api/sync/environments')
  if (!res.data?.success) return []
  return Array.isArray(res.data.data) ? res.data.data : []
}

/** Enabled environments only — the channel sync dialog's target list. */
export async function getEnabledSyncEnvironments(): Promise<SyncEnvironment[]> {
  const res = await api.get('/api/sync/environments/enabled')
  if (!res.data?.success) return []
  return Array.isArray(res.data.data) ? res.data.data : []
}

export async function createSyncEnvironment(
  payload: SyncEnvironmentPayload
): Promise<ApiResponse> {
  const res = await api.post('/api/sync/environments', payload)
  return res.data
}

export async function updateSyncEnvironment(
  id: number,
  payload: SyncEnvironmentPayload
): Promise<ApiResponse> {
  const res = await api.put(`/api/sync/environments/${id}`, payload)
  return res.data
}

export async function deleteSyncEnvironment(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/sync/environments/${id}`)
  return res.data
}

/** Verifies the stored credentials against the remote deployment. */
export async function testSyncEnvironment(id: number): Promise<ApiResponse> {
  const res = await api.post(`/api/sync/environments/${id}/test`)
  return res.data
}

export async function getSyncLogs(params: {
  page: number
  page_size: number
}): Promise<{ items: SyncLog[]; total: number }> {
  const res = await api.get('/api/sync/logs', { params })
  if (!res.data?.success) return { items: [], total: 0 }
  return {
    items: Array.isArray(res.data.data) ? res.data.data : [],
    total: res.data.total ?? 0,
  }
}

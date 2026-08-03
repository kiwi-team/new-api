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
  ChannelNameOption,
  ModelRouteConfigListResponse,
  ModelRouteConfigPayload,
} from './types'

type ListParams = {
  p: number
  page_size: number
  keyword?: string
  model_keyword?: string
  channel_id?: number
}

export async function getModelRouteConfigs(
  params: ListParams
): Promise<ModelRouteConfigListResponse> {
  const { keyword, model_keyword, channel_id, ...page } = params
  const hasFilter = Boolean(keyword || model_keyword || channel_id)
  const query = {
    ...page,
    ...(keyword ? { keyword } : {}),
    ...(model_keyword ? { model_keyword } : {}),
    ...(channel_id ? { channel_id } : {}),
  }
  const res = await api.get(
    hasFilter ? '/api/model_route_config/search' : '/api/model_route_config/',
    { params: query }
  )
  return res.data
}

export async function createModelRouteConfig(
  payload: ModelRouteConfigPayload
): Promise<ApiResponse> {
  const res = await api.post('/api/model_route_config/', payload)
  return res.data
}

export async function updateModelRouteConfig(
  payload: ModelRouteConfigPayload & { id: number }
): Promise<ApiResponse> {
  const res = await api.put('/api/model_route_config/', payload)
  return res.data
}

export async function deleteModelRouteConfig(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/model_route_config/${id}`)
  return res.data
}

export async function updateModelRouteConfigStatus(
  id: number,
  enabled: number
): Promise<ApiResponse> {
  const res = await api.post('/api/model_route_config/status', { id, enabled })
  return res.data
}

/** Channel id → name lookup used by the group editor and the list badges. */
export async function getChannelNameList(): Promise<ChannelNameOption[]> {
  const res = await api.get('/api/channel/channel-name-list')
  if (!res.data?.success) return []
  return Array.isArray(res.data.data) ? res.data.data : []
}

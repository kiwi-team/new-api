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

/** A channel in the target environment that could receive the sync. */
export type ChannelMatchInfo = {
  id: number
  name: string
  models: string
  type: number
}

export type ChannelPreviewItem = {
  channel_id: number
  channel_name: string
  models: string
  matches: ChannelMatchInfo[]
}

export type EnvironmentPreview = {
  environment_id: number
  environment_name: string
  channels: ChannelPreviewItem[]
  error?: string
}

export type SyncChannelDetail = {
  channel_id: number
  channel_name: string
  /** `created` or `updated` */
  action: string
  success: boolean
  error?: string
}

export type SyncResult = {
  environment_id: number
  environment_name: string
  success: boolean
  error?: string
  synced_count?: number
  details?: SyncChannelDetail[]
}

/**
 * Target channel chosen per environment:
 * `environment_id -> source_channel_id -> target_channel_id`.
 * A target of 0 means "create a new channel there".
 */
export type ChannelSyncMapping = Record<number, Record<number, number>>

export type ApiResponse<T = unknown> = {
  success: boolean
  message?: string
  data?: T
}

/** Ask each target environment which channels would match the selection. */
export async function previewChannelSync(params: {
  channel_ids: number[]
  environment_ids: number[]
}): Promise<ApiResponse<EnvironmentPreview[]>> {
  const res = await api.post('/api/sync/channels/preview', params)
  return res.data
}

export async function syncChannels(params: {
  channel_ids: number[]
  environment_ids: number[]
  channel_mapping?: ChannelSyncMapping
}): Promise<ApiResponse<SyncResult[]>> {
  const res = await api.post('/api/sync/channels', params)
  return res.data
}

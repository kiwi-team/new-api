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

/** How a group's channels are picked: in order, or at random. */
export type RouteRandomType = 'order' | 'random'

/**
 * One routing rule. The backend stores the pattern lists and channel groups
 * as JSON strings but marshals them back as arrays, so the client always
 * sees arrays.
 */
export type ModelRouteConfig = {
  id: number
  name: string
  model_patterns: string[]
  body_patterns: string[]
  url_patterns: string[]
  /** Groups tried in order; each group holds interchangeable channel ids */
  channel_groups: number[][]
  random_type: RouteRandomType
  max_retry: number
  priority: number
  /** 1 enabled, 0 disabled */
  enabled: number
  created_time?: number
  updated_time?: number
}

export type ModelRouteConfigPayload = {
  id?: number
  name: string
  model_patterns: string[]
  body_patterns: string[]
  url_patterns: string[]
  channel_groups: number[][]
  random_type: RouteRandomType
  max_retry: number
  priority: number
  enabled: number
}

export type ChannelNameOption = {
  id: number
  name: string
}

export type ApiResponse<T = unknown> = {
  success: boolean
  message?: string
  data?: T
}

export type ModelRouteConfigListResponse = ApiResponse<{
  items: ModelRouteConfig[]
  total: number
  p: number
  page_size: number
}>

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

/** A remote deployment this instance can push channels and prices to. */
export type SyncEnvironment = {
  id: number
  name: string
  api_url: string
  /** Write-only: the API never returns the stored token */
  root_token?: string
  new_api_user: string
  /** 1 enabled, 2 disabled */
  status: number
  remark?: string
  created_time?: number
  updated_time?: number
}

export type SyncEnvironmentPayload = {
  name: string
  api_url: string
  root_token?: string
  new_api_user: string
  status: number
  remark?: string
}

/** One past sync run against one environment. */
export type SyncLog = {
  id: number
  /** `channel` or `model_price` */
  sync_type: string
  environment_id: number
  environment_name: string
  data_summary: string
  /** 1 success, 2 failure */
  status: number
  error_message?: string
  created_time: number
  operator_id?: number
}

export type ApiResponse<T = unknown> = {
  success: boolean
  message?: string
  data?: T
  total?: number
}

export const SYNC_ENVIRONMENT_STATUS = {
  ENABLED: 1,
  DISABLED: 2,
} as const

export const SYNC_LOG_STATUS = {
  SUCCESS: 1,
  FAILED: 2,
} as const

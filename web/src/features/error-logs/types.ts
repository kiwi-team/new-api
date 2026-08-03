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

/** One relay failure recorded in the log database. */
export type ErrorLog = {
  id: number
  user_id: number
  created_at: number
  channel_id: number
  channel_name: string
  token_id: number
  token_name: string
  model_name: string
  message: string
  type: string
  param: string
  code: string
  request_id: string
  status_code: number
  use_time_ms: number
  ip: string
  client_user_id: string
  client_scenairo: string
  session_id: string
  /** JSON blob with nested ids (mt_session_id, trace_id, traj_id) */
  extra?: string | null
  /** Request headers; the API strips this for non-root callers */
  header?: string | null
}

export type ErrorLogFilters = {
  start_timestamp?: number
  end_timestamp?: number
  model_name?: string
  request_id?: string
  channel?: number
  token_id?: number
  client_user_id?: string
  mt_session_id?: string
  trace_id?: string
  traj_id?: string
  session_id?: string
}

export type ErrorLogListResponse = {
  success: boolean
  message?: string
  data?: {
    items: ErrorLog[]
    total: number
    page: number
    page_size: number
  }
}

export type ApiResponse<T = unknown> = {
  success: boolean
  message?: string
  data?: T
}

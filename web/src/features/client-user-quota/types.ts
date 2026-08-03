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
/** Budget record of one downstream client UID (`cliend_user_quotas`). */
export type ClientUserQuota = {
  id: number
  client_user_id: string
  /** Customer label, admin-only */
  client_name?: string
  fixed_quota: number
  temp_quota: number
  /** Quota units consumed this month */
  used_quota: number
  /** Unix seconds after which the temporary budget stops applying */
  expired_at?: number | null
  remark?: string
}

export type ClientUserQuotaPayload = {
  client_user_id: string
  client_name?: string
  fixed_quota: number
  temp_quota: number
  expired_at?: number | null
  remark?: string
}

/** One project's slice of a client UID's budget. */
export type ProjectAllocation = {
  project_id: number
  project_name: string
  allocated_quota: number
  used_quota_usd?: number | string
}

/** Rollup shown inline in the list, keyed by client UID. */
export type ProjectBudgetSummary = {
  total_allocated: number
  projects: { project_id: number; project_name: string; allocated_quota: number }[]
}

export type ApiResponse<T = unknown> = {
  success: boolean
  message?: string
  data?: T
}

export type ClientUserQuotaListResponse = ApiResponse<{
  items: ClientUserQuota[]
  total: number
  p: number
  page_size: number
}>

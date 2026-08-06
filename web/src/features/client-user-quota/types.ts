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

/**
 * One project allocation behind a client UID, as returned by
 * `/api/cliend_user_quota/project-allocations`.
 *
 * "Currently effective" needs all three of: the project is enabled, the plan is
 * the project's active plan, and today falls inside the plan's date range. The
 * three flags are kept separate so the UI can explain *why* an allocation is
 * not effective.
 */
export type ProjectAllocation = {
  allocation_id: number
  project_id: number
  project_name: string
  /** 1 = enabled, 2 = paused */
  project_status: number
  plan_id: number
  plan_name: string
  /** `YYYYMMDD`, empty when the plan has no bound */
  start_date: string
  end_date: string
  allocated_quota: number
  used_quota_usd: number
  remaining_quota_usd: number
  is_active_plan: boolean
  is_in_date_range: boolean
  is_current_effective: boolean
}

/**
 * Rollup shown inline in the list, keyed by client UID. Only currently
 * effective allocations are counted; history, paused, not-yet-started and
 * expired plans are excluded.
 */
export type ProjectBudgetSummary = {
  client_user_id: string
  total_allocated: number
  total_used_usd: number
  total_remaining_usd: number
  /** Project-attributed spend this month */
  monthly_project_used_usd: number
  /** Spend this month with no project tag — the figure the non-project budget gate compares against */
  monthly_non_project_used_usd: number
  project_count: number
  projects: ProjectAllocation[]
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

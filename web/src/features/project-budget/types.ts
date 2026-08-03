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
/** Allocation of a plan's budget to one client UID. */
export type PlanAllocation = {
  id: number
  client_user_id: string
  allocated_quota: number
  /** Consumption in quota units */
  used_quota: number
  created_at?: number
}

/** Plan summary embedded in the project list rows. */
export type ProjectPlanSummary = {
  plan_id: number
  plan_name: string
  /** `YYYYMMDD` */
  start_date: string
  end_date: string
  is_active: boolean
  allocations?: { client_user_id: string; allocated_quota: number }[]
}

/** Full plan record returned by `/api/project/:id/plans`. */
export type ProjectPlan = {
  id: number
  plan_name: string
  start_date: string
  end_date: string
  is_active: boolean
  is_expired: boolean
  allocated_total: number
  allocations?: { client_user_id: string; allocated_quota: number }[]
}

export type Project = {
  id: number
  project_name: string
  total_budget: number
  /** Consumption in quota units */
  quota: number
  allocated_total: number
  status: number
  active_plan_id?: number
  plans?: ProjectPlanSummary[]
}

export type ProjectDashboard = {
  projects?: {
    id: number
    project_name: string
    total_budget: number
    allocated_total: number
    /** Consumption in quota units */
    used_total: number
  }[]
}

export type ApiResponse<T = unknown> = {
  success: boolean
  message?: string
  data?: T
}

export type PagedResponse<T> = ApiResponse<{
  items: T[]
  total: number
  p: number
  page_size: number
}>

export type UidOption = {
  value: string
  label: string
}

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
  PagedResponse,
  PlanAllocation,
  Project,
  ProjectDashboard,
  ProjectPlan,
  UidOption,
} from './types'

// ============================================================================
// Projects
// ============================================================================

export async function getProjects(params: {
  p: number
  page_size: number
  keyword?: string
}): Promise<PagedResponse<Project>> {
  const { keyword, ...pageParams } = params
  const trimmed = keyword?.trim()
  const res = await api.get('/api/projects', {
    params: trimmed ? { ...pageParams, keyword: trimmed } : pageParams,
  })
  return res.data
}

export async function getProjectDashboard(): Promise<ProjectDashboard> {
  const res = await api.get('/api/project/dashboard')
  if (!res.data?.success) return {}
  return (res.data.data ?? {}) as ProjectDashboard
}

export async function createProject(payload: {
  project_name: string
  total_budget: number
}): Promise<ApiResponse> {
  const res = await api.post('/api/project', payload)
  return res.data
}

export async function updateProject(
  id: number,
  payload: { project_name: string; total_budget: number }
): Promise<ApiResponse> {
  const res = await api.put(`/api/project/${id}`, payload)
  return res.data
}

export async function updateProjectStatus(
  id: number,
  status: number
): Promise<ApiResponse> {
  const res = await api.put(`/api/project/${id}/status`, { status })
  return res.data
}

/** `planId = 0` clears the active plan. */
export async function setActivePlan(
  projectId: number,
  planId: number
): Promise<ApiResponse> {
  const res = await api.put(`/api/project/${projectId}/active-plan`, {
    plan_id: planId,
  })
  return res.data
}

// ============================================================================
// Plans
// ============================================================================

export async function getProjectPlans(
  projectId: number
): Promise<ProjectPlan[]> {
  const res = await api.get(`/api/project/${projectId}/plans`)
  if (!res.data?.success) return []
  return Array.isArray(res.data.data) ? res.data.data : []
}

export async function createProjectPlan(
  projectId: number,
  payload: { plan_name: string; start_date: string; end_date: string }
): Promise<ApiResponse> {
  const res = await api.post(`/api/project/${projectId}/plan`, payload)
  return res.data
}

export async function updateProjectPlan(
  planId: number,
  payload: { plan_name: string; start_date: string; end_date: string }
): Promise<ApiResponse> {
  const res = await api.put(`/api/project/plan/${planId}`, payload)
  return res.data
}

export async function deleteProjectPlan(planId: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/project/plan/${planId}`)
  return res.data
}

// ============================================================================
// Allocations (scoped to a plan)
// ============================================================================

export async function getPlanAllocations(
  planId: number,
  params: { p: number; page_size: number }
): Promise<PagedResponse<PlanAllocation>> {
  const res = await api.get(`/api/project/plan/${planId}/allocations`, {
    params,
  })
  return res.data
}

export async function upsertPlanAllocation(
  planId: number,
  payload: { client_user_id: string; allocated_quota: number }
): Promise<ApiResponse> {
  const res = await api.post(`/api/project/plan/${planId}/allocation`, payload)
  return res.data
}

/** Sets the allocated quota to 0, releasing the remaining budget. */
export async function clearAllocationBudget(
  allocationId: number
): Promise<ApiResponse> {
  const res = await api.post(`/api/project/allocation/${allocationId}/clear`)
  return res.data
}

/** Client UID picker source, shared with the UID budget page's records. */
export async function searchUidOptions(keyword: string): Promise<UidOption[]> {
  const trimmed = keyword.trim()
  const params = { p: 1, page_size: 50 }
  const res = trimmed
    ? await api.get('/api/cliend_user_quota/search', {
        params: { ...params, keyword: trimmed },
      })
    : await api.get('/api/cliend_user_quota/', { params })
  if (!res.data?.success) return []
  const items = (res.data.data?.items ?? []) as {
    client_user_id: string
    client_name?: string
  }[]
  return items.map((item) => ({
    value: item.client_user_id,
    label: item.client_name
      ? `${item.client_user_id} (${item.client_name})`
      : item.client_user_id,
  }))
}

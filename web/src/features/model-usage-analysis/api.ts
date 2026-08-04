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
  CreateUsageAdjustmentPayload,
  ModelUsageRow,
  UsageHourSnapshot,
} from './types'

export async function getModelUsageAnalysis(params: {
  start_timestamp: number
  end_timestamp: number
}): Promise<ModelUsageRow[]> {
  const res = await api.get('/api/data/model-usage-analysis', { params })
  if (!res.data?.success) {
    throw new Error(res.data?.message || 'Failed to load')
  }
  return Array.isArray(res.data.data) ? res.data.data : []
}

export async function getUsageHourSnapshot(params: {
  hour_start: number
  token_id: number
  model_name: string
}): Promise<UsageHourSnapshot> {
  const res = await api.get('/api/data/model-usage-analysis/adjustments/hour', {
    params,
  })
  if (!res.data?.success) {
    throw new Error(res.data?.message || 'Failed to load usage hour')
  }
  return res.data.data as UsageHourSnapshot
}

export async function createUsageAdjustment(
  payload: CreateUsageAdjustmentPayload
): Promise<UsageHourSnapshot> {
  const res = await api.post(
    '/api/data/model-usage-analysis/adjustments',
    payload
  )
  if (!res.data?.success) {
    throw new Error(res.data?.message || 'Failed to create adjustment')
  }
  return res.data.data as UsageHourSnapshot
}

export async function revertUsageAdjustment(
  id: number,
  reason: string
): Promise<void> {
  const res = await api.post(
    `/api/data/model-usage-analysis/adjustments/${id}/revert`,
    { reason }
  )
  if (!res.data?.success) {
    throw new Error(res.data?.message || 'Failed to revert adjustment')
  }
}

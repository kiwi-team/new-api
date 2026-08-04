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
/** One aggregate row: date × key × model. */
export type ModelUsageRow = {
  date: string
  token_id?: number
  token_name?: string
  model_name?: string
  total_requests: number
  cache_write_requests: number
  cache_write_5m_requests: number
  cache_write_1h_requests: number
  cache_read_requests: number
  cache_write_tokens: number
  cache_read_tokens: number
  input_tokens: number
  output_tokens: number
  avg_first_token_ms: number
  avg_use_time_ms: number
  cost_usd: number
}

/** Totals across the rows currently in view. */
export type ModelUsageSummary = {
  costUsd: number
  totalRequests: number
  cacheWriteRequests: number
  cacheReadRequests: number
  cacheWriteTokens: number
  cacheReadTokens: number
  inputTokens: number
  outputTokens: number
}

export type UsageHourValues = {
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_5m_tokens: number
  cache_write_1h_tokens: number
  quota: number
  cost_usd: number
}

export type UsageAdjustment = {
  id: number
  user_id: number
  hour_start: number
  token_id: number
  token_name: string
  model_name: string
  prompt_tokens_delta: number
  completion_tokens_delta: number
  cached_tokens_delta: number
  claude_cache_creation_5m_tokens_delta: number
  claude_cache_creation_1h_tokens_delta: number
  quota_delta: number
  reason: string
  ticket: string
  operator_id: number
  operator_name: string
  created_at: number
  reverted_at: number
  reverted_by: number
  reverted_by_name: string
  revert_reason: string
}

export type UsageHourSnapshot = {
  hour_start: number
  hour_end: number
  user_id: number
  token_id: number
  token_name: string
  model_name: string
  original: UsageHourValues
  effective: UsageHourValues
  adjustments: UsageAdjustment[]
}

export type CreateUsageAdjustmentPayload = {
  hour_start: number
  token_id: number
  model_name: string
  correct_input_tokens: number
  correct_output_tokens: number
  correct_cache_read_tokens: number
  correct_cache_write_5m_tokens: number
  correct_cache_write_1h_tokens: number
  correct_cost_usd: number
  reason: string
  ticket: string
}

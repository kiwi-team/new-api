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

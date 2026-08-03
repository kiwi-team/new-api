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
/**
 * One aggregated consumption row. Which of `date` / `model_name` /
 * `token_*` are present depends on the expand flags of the query.
 */
export type QuotaStatisticsRow = {
  client_user_id: string
  date?: string
  model_name?: string
  token_id?: number
  token_name?: string
  /** Monthly budget of the UID, used to colour the consumption cell */
  fixed_quota?: number
  temp_quota?: number
  total_quota: number
  total_count: number
  total_prompt: number
  total_completion: number
  total_cached_tokens?: number
  total_cache_cost?: number
  total_cache_creation_5m_tokens?: number
  total_cache_creation_5m_cost?: number
  total_cache_creation_1h_tokens?: number
  total_cache_creation_1h_cost?: number
}

export type QuotaStatisticsQuery = {
  start_timestamp: number
  end_timestamp: number
  model_name?: string
  client_user_id?: string
  client_scenairos?: string
  expand_models: boolean
  expand_dates: boolean
  expand_tokens: boolean
  /** Root only: look at another user's consumption */
  user_id?: number
  project_name?: string
  token_ids?: string
}

export type TokenOption = {
  id: number
  name: string
  key?: string
}

export type ApiResponse<T = unknown> = {
  success: boolean
  message?: string
  data?: T
}

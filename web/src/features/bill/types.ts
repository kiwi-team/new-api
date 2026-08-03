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
/** One row of the settlement bill, per model (and per date when expanded). */
export type BillItem = {
  model_name: string
  /** Present only when the query was expanded by date (`YYYY-MM-DD`) */
  date?: string
  input_tokens: number
  output_tokens: number
  request_count: number
  input_amount: number
  output_amount: number
  request_amount: number
  total_amount: number
  /** False when the model has no settlement price configured for this account */
  configured: boolean
}

export type Bill = {
  items: BillItem[]
  total_amount: number
  expand_date?: boolean
}

export type BillQueryParams = {
  start_timestamp: number
  end_timestamp: number
  token_id?: number
  expand_date?: boolean
}

/** Key option of the bill filter; admins see other users' keys too. */
export type BillTokenOption = {
  id: number
  name?: string
  username?: string
}

/**
 * The caller's own settlement prices, plus the site-wide ratios used to show
 * what the undiscounted price would be.
 */
export type SelfSettlementConfig = {
  id: number
  model_name: string
  discount: number
  input_price: number
  output_price: number
  request_price: number
  site_configured?: boolean
  site_is_per_call?: boolean
  site_model_ratio?: number
  site_completion_ratio?: number
}

export type ApiResponse<T = unknown> = {
  success: boolean
  message?: string
  data?: T
}

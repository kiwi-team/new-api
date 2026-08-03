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
 * Per-customer, per-model settlement override (`settlement_configs`).
 *
 * The same records back two screens: this page (one customer at a time) and
 * the Pricing Center's customer-discount tab (aggregated by model).
 */
export type SettlementConfig = {
  id: number
  user_id: number
  username?: string
  model_name: string
  input_price: number
  output_price: number
  request_price: number
  discount: number
  created_at?: number
  updated_at?: number
}

export type SettlementConfigPayload = {
  user_id: number
  model_name: string
  discount: number
  input_price: number
  output_price: number
  request_price: number
}

export type UserOption = {
  value: number
  label: string
}

export type ApiResponse<T = unknown> = {
  success: boolean
  message?: string
  data?: T
}

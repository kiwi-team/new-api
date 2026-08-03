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
/** Discount bounds mirror `SettlementDiscountMin/Max` in the backend model. */
export const DISCOUNT_MIN = 0.01
export const DISCOUNT_MAX = 10

/** Placeholder shown in the batch import dialog. */
export const BATCH_IMPORT_EXAMPLE = `{
  "user_id": 1,
  "configs": [
    {"model_name": "gpt-4o*", "discount": 0.8, "input_price": 2.5, "output_price": 10.0, "request_price": 0},
    {"model_name": "gpt-image-1", "discount": 1, "input_price": 0, "output_price": 0, "request_price": 0.02}
  ]
}`

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
import type { TFunction } from 'i18next'

import type { QuotaStatisticsRow } from './types'

/** Client-declared usage scenario recorded on each request. */
export const SCENARIO_VALUES = [
  'PersonalExperiment',
  'ReleaseEvaluation',
  'DailyExternalModelEvaluation',
  'Other',
] as const

export function getScenarioLabel(value: string, t: TFunction): string {
  switch (value) {
    case 'PersonalExperiment':
      return t('Personal experiment (incl. untagged)')
    case 'ReleaseEvaluation':
      return t('Release evaluation')
    case 'DailyExternalModelEvaluation':
      return t('Daily external model evaluation')
    default:
      return t('Other')
  }
}

/** The default window is "this month so far". */
export function currentMonthRange(): { start: Date; end: Date } {
  const now = new Date()
  return {
    start: new Date(now.getFullYear(), now.getMonth(), 1, 0, 0, 0, 0),
    end: new Date(
      now.getFullYear(),
      now.getMonth(),
      now.getDate(),
      23,
      59,
      59,
      999
    ),
  }
}

/** Cache-creation tokens are only reported for Claude models. */
export function isClaudeModel(modelName: string | undefined): boolean {
  return String(modelName ?? '')
    .toLowerCase()
    .includes('claude')
}

/**
 * Consumption is highlighted once it eats into the UID's monthly budget:
 * amber past half of it, red once the budget is used up.
 */
export function budgetUsageLevel(
  row: QuotaStatisticsRow
): 'none' | 'warning' | 'exceeded' {
  const budget =
    (Number(row.fixed_quota) || 0) + (Number(row.temp_quota) || 0)
  if (budget <= 0) return 'none'
  const consumed = Number(row.total_quota) || 0
  if (consumed >= budget) return 'exceeded'
  if (consumed >= budget * 0.5) return 'warning'
  return 'none'
}

export function statisticsRowKey(row: QuotaStatisticsRow): string {
  return [
    row.client_user_id ?? '',
    row.date ?? '',
    row.model_name ?? '',
    row.token_id ?? '',
  ].join('_')
}

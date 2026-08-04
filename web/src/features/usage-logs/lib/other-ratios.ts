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
 * Human-readable label and value for one entry of `other.other_ratios`.
 *
 * The backend writes these for task-style models (video and friends) from
 * `PriceData.OtherRatios` — typically `{ seconds: 8, 'resolution-1080P': 1.67 }`.
 * They already multiplied into the charge, so the breakdown has to show them:
 * an 8-second video is billed 8x, and without this row the displayed factors
 * multiply out to an eighth of the real amount.
 *
 * Ratios equal to 1 are dropped, matching the backend, which only folds
 * non-unit ratios into the product — rendering them would add noise like "x1".
 */
export type OtherRatioEntry = {
  key: string
  label: string
  value: number
}

const RESOLUTION_KEY = /^resolution-(.+)$/

export function formatOtherRatios(
  t: (key: string, opts?: Record<string, unknown>) => string,
  otherRatios: Record<string, number> | undefined
): OtherRatioEntry[] {
  if (!otherRatios) return []

  const knownLabels: Record<string, string> = {
    seconds: t('Duration (s)'),
    size: t('Size'),
    n: t('Count'),
  }

  return Object.entries(otherRatios)
    .filter(([, value]) => Number.isFinite(value) && value !== 1)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => {
      const resolution = RESOLUTION_KEY.exec(key)
      return {
        key,
        label:
          knownLabels[key] ??
          (resolution
            ? t('Resolution ({{value}})', { value: resolution[1] })
            : key),
        value,
      }
    })
}

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
import type { SyncResult } from '@/features/channels/lib/channel-sync'
import { api } from '@/lib/api'

export type ModelPriceSyncResult = SyncResult

/**
 * Push model pricing to other deployments.
 *
 * An empty `selected_models` means "sync everything", which replaces the
 * target's pricing wholesale; a non-empty list is merged into what the target
 * already has (`SyncModelPricesIncrementalToEnvironment`). The two modes are
 * meaningfully different, so the dialog makes the choice explicit.
 *
 * Note this endpoint returns its payload under `results`, not the usual
 * `data`.
 */
export async function syncModelPrices(params: {
  environment_ids: number[]
  selected_models?: string[]
}): Promise<{
  success: boolean
  message?: string
  results?: ModelPriceSyncResult[]
}> {
  const res = await api.post('/api/sync/model-prices', params)
  return res.data
}

/**
 * Every model that has any pricing configured locally.
 *
 * Derived from the three option blobs the sync sends, so the picker offers
 * exactly what can be pushed.
 */
export function collectPricedModels(options: {
  ModelPrice?: string
  ModelRatio?: string
  CompletionRatio?: string
}): string[] {
  const names = new Set<string>()

  for (const raw of [
    options.ModelPrice,
    options.ModelRatio,
    options.CompletionRatio,
  ]) {
    if (!raw) continue
    try {
      const parsed = JSON.parse(raw) as Record<string, unknown>
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        Object.keys(parsed).forEach((name) => names.add(name))
      }
    } catch {
      // A malformed blob just contributes nothing to the picker.
    }
  }

  return [...names].sort()
}

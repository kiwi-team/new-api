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
import assert from 'node:assert/strict'
import { after, describe, test } from 'node:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Window } from 'happy-dom'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'MouseEvent',
  'PointerEvent',
  'MutationObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
  'ResizeObserver',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { ModelUsageAnalysisPage } = await import('../index')
const { getDefaultUsageDateRange, usageDateRangeTimestamps } =
  await import('../lib')
const { api } = await import('@/lib/http-client')
const { useAuthStore } = await import('@/stores/auth-store')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: { Refresh: 'Refresh' } } },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const dates = getDefaultUsageDateRange()
const range = usageDateRangeTimestamps(dates.start, dates.end)

function usageRow(costUsd: number) {
  return {
    date: dates.end,
    token_id: 7,
    token_name: 'key',
    model_name: 'model',
    total_requests: 1,
    cache_write_requests: 0,
    cache_write_5m_requests: 0,
    cache_write_1h_requests: 0,
    cache_read_requests: 0,
    cache_write_tokens: 0,
    cache_read_tokens: 0,
    input_tokens: 10,
    output_tokens: 5,
    avg_first_token_ms: 0,
    avg_use_time_ms: 1,
    cost_usd: costUsd,
  }
}

/**
 * Stub the axios adapter rather than the feature's own api module: the network
 * is the only boundary worth faking here, and the component -> api.ts -> axios
 * path stays real so a missing request shows up as a missing call.
 */
async function render() {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'tester', role: 100 })
  const requestedRanges: string[] = []
  const originalAdapter = api.defaults.adapter
  api.defaults.adapter = async (config) => {
    const isUsageRequest = config.url?.includes(
      '/api/data/model-usage-analysis'
    )
    if (isUsageRequest) {
      requestedRanges.push(String(config.params?.start_timestamp))
    }
    return {
      data: isUsageRequest
        ? { success: true, data: [usageRow(0.02)] }
        : { success: true, data: { items: [] } },
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
    }
  }

  const queryClient = new QueryClient({
    // staleTime Infinity is the production-like case the refresh button has to
    // defeat: cached data is never considered stale on its own.
    defaultOptions: { queries: { staleTime: Infinity, retry: false } },
  })
  queryClient.setQueryData(['model-usage-analysis', range], [usageRow(0.01)])

  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  await act(async () => {
    root.render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <ModelUsageAnalysisPage />
        </I18nextProvider>
      </QueryClientProvider>
    )
  })

  return {
    container,
    requestedRanges,
    async clickRefresh() {
      const button = [...container.querySelectorAll('button')].find((el) =>
        el.textContent?.includes('Refresh')
      )
      assert.ok(button, 'refresh button should be rendered')
      await act(async () => button.click())
    },
    async cleanup() {
      await act(async () => root.unmount())
      queryClient.clear()
      container.remove()
      api.defaults.adapter = originalAdapter
    },
  }
}

describe('model usage analysis refresh', () => {
  after(() => domWindow.close())

  test('refetches usage rows when refresh is clicked with an unchanged date range', async () => {
    const rendered = await render()
    assert.deepEqual(rendered.requestedRanges, [])

    await rendered.clickRefresh()

    assert.deepEqual(rendered.requestedRanges, [String(range.start_timestamp)])
    assert.ok(rendered.container.textContent?.includes('$0.02'))
    await rendered.cleanup()
  })
})

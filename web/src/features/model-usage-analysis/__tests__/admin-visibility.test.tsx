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
const { useAuthStore } = await import('@/stores/auth-store')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        Correct: 'Correct',
        'Correct hourly usage': 'Correct hourly usage',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

async function renderForRole(role: number) {
  useAuthStore.getState().auth.setUser({ id: role, username: 'tester', role })
  const queryClient = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity, retry: false } },
  })
  const dates = getDefaultUsageDateRange()
  const range = usageDateRangeTimestamps(dates.start, dates.end)
  queryClient.setQueryData(
    ['model-usage-analysis', range],
    [
      {
        date: dates.end,
        user_id: 8,
        username: 'customer',
        token_id: 7,
        token_name: 'key',
        model_name: 'model',
        active_hours: [9, 11],
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
        cost_usd: 0.01,
      },
    ]
  )
  queryClient.setQueryData(
    ['model-usage-analysis', 'user-options', ''],
    [{ value: 8, label: 'customer (ID: 8)' }]
  )
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
  return { container, root, queryClient }
}

async function cleanup(rendered: Awaited<ReturnType<typeof renderForRole>>) {
  await act(async () => rendered.root.unmount())
  rendered.queryClient.clear()
  rendered.container.remove()
}

describe('usage correction visibility', () => {
  after(() => domWindow.close())

  test('hides correction actions from regular users', async () => {
    const rendered = await renderForRole(1)
    const hasCorrectButton = [
      ...rendered.container.querySelectorAll('button'),
    ].some((button) => button.textContent?.includes('Correct'))
    assert.equal(hasCorrectButton, false)
    assert.equal(
      rendered.container.querySelector('[aria-label="Filter by user"]'),
      null
    )
    await cleanup(rendered)
  })

  test('shows correction actions to admin users', async () => {
    const rendered = await renderForRole(10)
    const hasCorrectButton = [
      ...rendered.container.querySelectorAll('button'),
    ].some((button) => button.textContent?.includes('Correct'))
    assert.equal(hasCorrectButton, true)
    assert.ok(
      rendered.container.querySelector('[aria-label="Filter by user"]'),
      'admin users should see the customer filter'
    )
    assert.ok(rendered.container.textContent?.includes('customer'))
    await cleanup(rendered)
  })

  // SectionPageLayout only renders children that match one of its slot
  // components, so a dialog placed as its direct sibling is dropped and the
  // action button silently does nothing.
  test('opens the adjustment dialog when the action is clicked', async () => {
    const rendered = await renderForRole(10)
    const button = [...rendered.container.querySelectorAll('button')].find(
      (candidate) => candidate.textContent?.includes('Correct')
    )
    assert.ok(button, 'admin users should see the correction action')

    await act(async () => button.click())

    assert.ok(
      document.body.textContent?.includes('Correct hourly usage'),
      'clicking the correction action should open the dialog'
    )
    await cleanup(rendered)
  })

  // Offering all 24 hours made the dialog 404 on open, because a daily row only
  // holds raw usage for the few hours the backend reports in active_hours.
  test('offers only the hours that carry correctable usage', async () => {
    const rendered = await renderForRole(10)
    const button = [...rendered.container.querySelectorAll('button')].find(
      (candidate) => candidate.textContent?.includes('Correct')
    )
    assert.ok(button)
    await act(async () => button.click())

    const select = document.querySelector<HTMLSelectElement>(
      '#usage-adjustment-hour'
    )
    assert.ok(select, 'the hour picker should render')
    assert.deepEqual(
      [...select.querySelectorAll('option')].map((option) => option.value),
      ['9', '11']
    )
    assert.equal(select.value, '9', 'the first active hour is preselected')
    await cleanup(rendered)
  })
})

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
const { ErrorLogDetailSheet } = await import(
  '../components/error-log-detail-sheet'
)
const { api } = await import('@/lib/http-client')
const { useAuthStore } = await import('@/stores/auth-store')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Request body': 'Request body',
        'Request headers': 'Request headers',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

/**
 * Flush pending microtasks inside act() until the expected output appears.
 * No timers and no fixed waits: each pass only lets already-resolved promises
 * (the stubbed adapter, then react-query's notification) run to completion.
 */
async function flushUntil(predicate: () => boolean): Promise<void> {
  for (let pass = 0; pass < 50 && !predicate(); pass += 1) {
    await act(async () => {
      await Promise.resolve()
    })
  }
}

const errorLog = {
  id: 42,
  user_id: 3,
  created_at: 1_785_254_400,
  channel_id: 9,
  channel_name: 'upstream',
  token_id: 7,
  token_name: 'key',
  model_name: 'model',
  message: 'upstream refused',
  type: 'upstream_error',
  param: '',
  code: 'bad_gateway',
  request_id: 'req-abc',
  status_code: 502,
  use_time_ms: 1200,
  ip: '10.0.0.1',
  client_user_id: 'uid-1',
  client_scenairo: 'PersonalExperiment',
  session_id: 'sess-1',
}

/**
 * The body endpoint returns a bare string while the header endpoint wraps its
 * payload in `{ content }`, matching `/api/log/:id/header`. Both shapes are
 * served here so a mismatched unwrap surfaces as a render failure.
 */
async function renderSheet() {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'root', role: 100 })
  const originalAdapter = api.defaults.adapter
  api.defaults.adapter = async (config) => {
    const url = String(config.url)
    const data = url.endsWith('/header')
      ? { success: true, data: { content: '{"authorization":"redacted"}' } }
      : { success: true, data: '{"model":"model"}' }
    return { data, status: 200, statusText: 'OK', headers: {}, config }
  }

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const container = document.createElement('div')
  document.body.append(container)
  const renderErrors: unknown[] = []
  const root = createRoot(container, {
    onUncaughtError: (error: unknown) => renderErrors.push(error),
  })
  await act(async () => {
    root.render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <ErrorLogDetailSheet
            log={errorLog}
            onOpenChange={() => {}}
            canViewHeader
          />
        </I18nextProvider>
      </QueryClientProvider>
    )
  })
  // The sheet portals its content out of the mount node.
  const text = () => document.body.textContent ?? ''
  // Both the body and the header query have to leave their loading state.
  await flushUntil(() => !text().includes('Loading...'))

  return {
    renderErrors,
    text,
    async cleanup() {
      await act(async () => root.unmount())
      queryClient.clear()
      container.remove()
      api.defaults.adapter = originalAdapter
    },
  }
}

describe('error log detail sheet', () => {
  after(() => domWindow.close())

  test('renders the request header payload without throwing during render', async () => {
    const rendered = await renderSheet()

    assert.deepEqual(rendered.renderErrors, [])
    assert.ok(
      rendered.text().includes('redacted'),
      'the header content should be shown'
    )
    assert.ok(
      !rendered.text().includes('[object Object]'),
      'the header payload should not be stringified as an object'
    )

    await rendered.cleanup()
  })
})

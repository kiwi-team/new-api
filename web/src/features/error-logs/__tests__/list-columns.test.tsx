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

// happy-dom has no scroll implementation; the router restores scroll on load.
Object.defineProperty(globalThis, 'scrollTo', {
  configurable: true,
  value: () => {},
})

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} = await import('@tanstack/react-router')
// The page resolves its page number through the route api, so it needs its real
// route to be mounted. Re-parenting the file route onto a bare root keeps its
// `/_authenticated/error-logs/` id — the same trick routeTree.gen.ts uses —
// without dragging in the whole authenticated app shell.
const { Route: errorLogsRoute } = await import(
  '@/routes/_authenticated/error-logs/index'
)
const { api } = await import('@/lib/http-client')
const { useAuthStore } = await import('@/stores/auth-store')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Request ID': 'Request ID',
        'Session ID': 'Session ID',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const REQUEST_ID = 'req-3f9c'
const SESSION_ID = 'sess-should-not-be-listed'

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

async function renderList() {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  const originalAdapter = api.defaults.adapter
  api.defaults.adapter = async (config) => ({
    data: {
      success: true,
      data: {
        items: [
          {
            id: 11,
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
            request_id: REQUEST_ID,
            status_code: 502,
            use_time_ms: 1200,
            ip: '10.0.0.1',
            client_user_id: 'uid-1',
            client_scenairo: 'PersonalExperiment',
            session_id: SESSION_ID,
            extra: JSON.stringify({ mt_session_id: 'mt-should-not-be-listed' }),
          },
        ],
        total: 1,
        page: 1,
        page_size: 20,
      },
    },
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  })

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const rootRoute = createRootRoute()
  // Route ids compose from the parent, so the pathless `_authenticated` layout
  // has to exist for the page's `/_authenticated/error-logs/` id to resolve.
  const authenticatedRoute = createRoute({
    getParentRoute: () => rootRoute,
    id: '_authenticated',
  })
  // Re-parenting is cast the same way routeTree.gen.ts casts it: `id` and
  // `getParentRoute` sit outside the public UpdatableRouteOptions surface.
  errorLogsRoute.update({
    id: '/error-logs/',
    path: '/error-logs/',
    getParentRoute: () => authenticatedRoute,
  } as never)
  const router = createRouter({
    routeTree: rootRoute.addChildren([
      authenticatedRoute.addChildren([errorLogsRoute]),
    ]),
    history: createMemoryHistory({ initialEntries: ['/error-logs?page=1'] }),
  })
  await router.load()

  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  await act(async () => {
    root.render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <RouterProvider router={router} />
        </I18nextProvider>
      </QueryClientProvider>
    )
  })
  await flushUntil(() =>
    Boolean(container.querySelector('tbody')?.textContent?.includes(REQUEST_ID))
  )

  const table = container.querySelector('table')
  assert.ok(table, 'the error log table should be rendered')
  return {
    headers: [...table.querySelectorAll('thead th')].map((cell) =>
      cell.textContent?.trim()
    ),
    bodyText: table.querySelector('tbody')?.textContent ?? '',
    async cleanup() {
      await act(async () => root.unmount())
      queryClient.clear()
      container.remove()
      api.defaults.adapter = originalAdapter
    },
  }
}

describe('error log list columns', () => {
  after(() => domWindow.close())

  test('lists the request id instead of the session id', async () => {
    const rendered = await renderList()

    assert.ok(
      rendered.headers.includes('Request ID'),
      `expected a Request ID column, got ${rendered.headers.join(', ')}`
    )
    assert.ok(
      !rendered.headers.includes('Session ID'),
      `expected no Session ID column, got ${rendered.headers.join(', ')}`
    )
    assert.ok(rendered.bodyText.includes(REQUEST_ID))
    assert.ok(!rendered.bodyText.includes(SESSION_ID))
    assert.ok(!rendered.bodyText.includes('mt-should-not-be-listed'))

    await rendered.cleanup()
  })
})

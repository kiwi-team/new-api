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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import {
  afterAll,
  afterEach,
  beforeEach,
  expect,
  onTestFinished,
  test,
  vi,
} from 'vitest'

import en from '@/i18n/locales/en.json'
import { useAuthStore } from '@/stores/auth-store'

import type { UsageLog } from '../../data/schema'
import { DetailsDialog } from '../dialogs/details-dialog'

vi.mock('@lobehub/icons', () => ({}))
vi.hoisted(() => {
  vi.stubGlobal('localStorage', {
    getItem: () => null,
    setItem: () => undefined,
    removeItem: () => undefined,
  })
})
afterAll(() => vi.unstubAllGlobals())

const timingSnapshot = {
  schema_version: 1,
  phase: 'consume_log',
  request_id: 'req-1',
  request: {
    pre_upstream_ms: 397000,
    route_ms: 0,
    gateway_total_ms: null,
  },
  attempts: [{ attempt_id: 'attempt-1', channel_id: 1567 }],
}
const consumeLog: UsageLog = {
  id: 1,
  user_id: 1,
  created_at: 1,
  type: 2,
  content: '',
  username: 'user',
  token_name: 'token',
  model_name: 'timing-model',
  quota: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  use_time: 3,
  is_stream: true,
  channel: 1567,
  channel_name: 'channel',
  token_id: 1,
  group: 'default',
  ip: '',
  other: '',
  request_id: 'req-1',
  upstream_request_id: '',
}
let client: QueryClient
let i18n: ReturnType<typeof createInstance>
const previousAuth = useAuthStore.getState().auth
beforeEach(async () => {
  i18n = createInstance()
  await i18n.init({
    lng: 'en',
    resources: { en },
    interpolation: { escapeValue: false },
  })
  useAuthStore.setState({ auth: { ...previousAuth, user: null } })
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const updatedAt = Date.now() + 60_000
  client.setQueryData(['status'], {}, { updatedAt })
  client.setQueryData(['pricing'], { data: [], vendors: [] }, { updatedAt })
})
afterEach(() => {
  client.clear()
  useAuthStore.setState({ auth: previousAuth })
})

function renderConsumptionDetails(
  timing: Record<string, unknown> | null | undefined,
  isAdmin = true,
  isRoot = false
) {
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <DetailsDialog
          log={{
            ...consumeLog,
            other: JSON.stringify({ admin_info: { timing } }),
          }}
          isAdmin={isAdmin}
          isRoot={isRoot}
          open
          onOpenChange={() => undefined}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

test.each([
  { isAdmin: true, isRoot: false },
  { isAdmin: true, isRoot: true },
  { isAdmin: false, isRoot: false },
])(
  'consumption details show recorded timing only for admin=$isAdmin root=$isRoot',
  ({ isAdmin, isRoot }) => {
    renderConsumptionDetails(timingSnapshot, isAdmin, isRoot)
    if (!isAdmin) {
      expect(
        screen.queryByRole('group', { name: 'Request timing' })
      ).not.toBeInTheDocument()
      expect(screen.queryByText(/pre_upstream_ms/)).not.toBeInTheDocument()
      return
    }
    const timing = screen.getByRole('group', { name: 'Request timing' })
    expect(timing).toBeVisible()
    expect(within(timing).getByText('397 s')).toBeVisible()
    expect(within(timing).getByText('Pending at snapshot')).toBeVisible()
    fireEvent.click(within(timing).getByRole('tab', { name: 'JSON' }))
    expect(
      JSON.parse(timing.querySelector('pre')?.textContent || '{}')
    ).toEqual(timingSnapshot)
  }
)

test('the timing copy button copies the complete recorded snapshot', () => {
  const previousClipboard = Object.getOwnPropertyDescriptor(
    navigator,
    'clipboard'
  )
  const writeText = vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText },
  })
  onTestFinished(() => {
    if (previousClipboard) {
      Object.defineProperty(navigator, 'clipboard', previousClipboard)
    } else {
      Reflect.deleteProperty(navigator, 'clipboard')
    }
  })
  renderConsumptionDetails(timingSnapshot)
  const timing = within(screen.getByRole('group', { name: 'Request timing' }))
  fireEvent.click(timing.getByRole('button', { name: 'Copy JSON' }))
  expect(writeText).toHaveBeenCalledWith(
    JSON.stringify(timingSnapshot, null, 2)
  )
})

test.each([undefined, null])(
  'older consumption details without a timing snapshot (%s) omit the section',
  (timing) => {
    renderConsumptionDetails(timing)
    expect(
      screen.queryByRole('group', { name: 'Request timing' })
    ).not.toBeInTheDocument()
  }
)

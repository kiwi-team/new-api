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
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { QuotaStatisticsPage } from '../index'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        All: 'All',
        Channel: 'Channel',
      },
    },
  },
})

let client: QueryClient

function renderPage(role: number) {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'operator', role })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <QuotaStatisticsPage />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/data/channel-list') {
      return {
        data: {
          success: true,
          data: [{ id: 22, name: 'Primary channel', status: 1 }],
        },
      }
    }
    if (url === '/api/data/statistics') {
      return { data: { success: true, data: [] } }
    }
    if (url === '/api/data/project-names' || url === '/api/data/token-list') {
      return { data: { success: true, data: [] } }
    }
    if (url === '/api/user/') {
      return { data: { success: true, data: { items: [] } } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
})

afterEach(() => {
  cleanup()
  client?.clear()
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
})

test('root can select a channel and sends its id to consumption statistics', async () => {
  const user = userEvent.setup()
  renderPage(ROLE.SUPER_ADMIN)

  const channel = await screen.findByRole('combobox', { name: 'Channel' })
  expect(api.get).toHaveBeenCalledWith('/api/data/channel-list')
  await user.click(channel)
  await user.click(
    await screen.findByRole('option', { name: 'Primary channel (ID: 22)' })
  )

  await waitFor(() =>
    expect(api.get).toHaveBeenCalledWith(
      '/api/data/statistics',
      expect.objectContaining({
        params: expect.objectContaining({ channel_id: 22 }),
      })
    )
  )

  await user.click(
    within(channel.parentElement as HTMLElement).getByRole('button', {
      name: 'Clear',
    })
  )
  expect(channel).toHaveValue('')
  await user.click(screen.getByRole('button', { name: 'Query' }))
  await waitFor(() => {
    const statisticsCalls = vi
      .mocked(api.get)
      .mock.calls.filter(([url]) => url === '/api/data/statistics')
    const latestConfig = statisticsCalls.at(-1)?.[1]
    expect(latestConfig?.params).not.toHaveProperty('channel_id')
  })
})

test('admin neither sees the channel filter nor requests its options', async () => {
  renderPage(ROLE.ADMIN)

  await screen.findByText('Quota Statistics')
  expect(screen.queryByRole('combobox', { name: 'Channel' })).toBeNull()
  expect(api.get).not.toHaveBeenCalledWith('/api/data/channel-list')
})

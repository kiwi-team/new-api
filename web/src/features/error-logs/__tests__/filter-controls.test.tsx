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
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import type { AxiosResponse } from 'axios'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { ErrorLogsPage } from '..'

const routeMocks = vi.hoisted(() => ({ navigate: vi.fn() }))

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    getRouteApi: () => ({
      useSearch: () => ({ page: 1 }),
      useNavigate: () => routeMocks.navigate,
    }),
  }
})

const originalAdapter = api.defaults.adapter

function apiResponse(config: AxiosResponse['config'], data: unknown) {
  return {
    data,
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  }
}

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  render(
    <QueryClientProvider client={queryClient}>
      <ErrorLogsPage />
    </QueryClientProvider>
  )
}

describe('error log filter controls', () => {
  beforeEach(() => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'root',
      role: 100,
    })
    api.defaults.adapter = async (config) => {
      if (config.url === '/api/data/token-list') {
        return apiResponse(config, {
          success: true,
          data: [{ id: 7, name: 'Alpha Key' }],
        })
      }
      if (config.url === '/api/channel/channel-name-list') {
        return apiResponse(config, {
          success: true,
          data: [{ id: 9, name: 'Primary Channel' }],
        })
      }
      if (config.url?.startsWith('/api/cliend_user_quota')) {
        return apiResponse(config, {
          success: true,
          data: {
            items: [{ client_user_id: 'cust-42', client_name: 'Acme' }],
          },
        })
      }
      return apiResponse(config, {
        success: true,
        data: { items: [], total: 0, page: 1, page_size: 20 },
      })
    }
  })

  afterEach(() => {
    api.defaults.adapter = originalAdapter
    useAuthStore.getState().auth.reset()
  })

  test('channel, key, and client UID filters expose searchable options', async () => {
    renderPage()
    const channelInput = screen.getByRole('combobox', { name: 'Channel ID' })
    const keyInput = screen.getByRole('combobox', { name: 'Key ID' })
    const clientUidInput = screen.getByRole('combobox', { name: 'Client UID' })

    for (const input of [channelInput, keyInput, clientUidInput]) {
      expect(input).toHaveAttribute('aria-autocomplete', 'list')
    }

    fireEvent.focus(channelInput)
    fireEvent.change(channelInput, { target: { value: 'primary' } })
    expect(
      await screen.findByRole('option', { name: 'Primary Channel (ID: 9)' })
    ).toBeVisible()

    fireEvent.focus(keyInput)
    fireEvent.change(keyInput, { target: { value: 'alpha' } })
    expect(
      await screen.findByRole('option', { name: 'Alpha Key (ID: 7)' })
    ).toBeVisible()

    fireEvent.focus(clientUidInput)
    fireEvent.change(clientUidInput, { target: { value: 'cust' } })
    expect(
      await screen.findByRole('option', { name: 'cust-42 (Acme)' })
    ).toBeVisible()
  })

  test('a selected error-log filter can be cleared', async () => {
    renderPage()
    const channelInput = screen.getByRole('combobox', { name: 'Channel ID' })

    fireEvent.focus(channelInput)
    fireEvent.change(channelInput, { target: { value: '9' } })
    fireEvent.keyDown(channelInput, { key: 'Enter' })

    await waitFor(() => {
      expect(channelInput).toHaveValue('9')
    })
    const inputContainer = channelInput.parentElement
    expect(inputContainer).not.toBeNull()
    fireEvent.click(
      within(inputContainer as HTMLElement).getByRole('button', {
        name: 'Clear',
      })
    )

    expect(channelInput).toHaveValue('')
    expect(
      within(inputContainer as HTMLElement).queryByRole('button', {
        name: 'Clear',
      })
    ).toBeNull()
  })
})

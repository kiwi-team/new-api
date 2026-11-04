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
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import type { AxiosResponse } from 'axios'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { Route as errorLogsRoute } from '@/routes/_authenticated/error-logs/index'
import { useAuthStore } from '@/stores/auth-store'

const originalAdapter = api.defaults.adapter
const listRequests = vi.fn()
const queryClients: QueryClient[] = []
let logMessage: string

function apiResponse(config: AxiosResponse['config'], data: unknown) {
  return {
    data,
    status: 200,
    statusText: 'OK',
    headers: {},
    config,
  }
}

async function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  queryClients.push(queryClient)
  const rootRoute = createRootRoute()
  const authenticatedRoute = createRoute({
    getParentRoute: () => rootRoute,
    id: '_authenticated',
  })
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

  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('error log filter controls', () => {
  beforeEach(() => {
    logMessage = 'Initial upstream failure'
    vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
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
      if (config.url === '/api/log/error-logs') {
        listRequests(config.params)
        return apiResponse(config, {
          success: true,
          data: {
            items: logMessage
              ? [{ id: 1, created_at: 1_793_750_400, message: logMessage }]
              : [],
            total: logMessage ? 21 : 0,
            page: config.params.p,
            page_size: 20,
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
    cleanup()
    for (const queryClient of queryClients.splice(0)) queryClient.clear()
    vi.useRealTimers()
    api.defaults.adapter = originalAdapter
    useAuthStore.getState().auth.reset()
  })

  test('channel, key, and client UID filters expose searchable options', async () => {
    await renderPage()
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
    await renderPage()
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

  test('time filtering uses the common log compact range picker', async () => {
    await renderPage()

    const rangeTrigger = screen.getByRole('button', {
      name: /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} ~ \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/,
    })
    fireEvent.click(rangeTrigger)

    expect(screen.getByLabelText('Start Time')).toHaveAttribute(
      'type',
      'datetime-local'
    )
    expect(screen.getByLabelText('End Time')).toHaveAttribute(
      'type',
      'datetime-local'
    )
    expect(screen.getByRole('button', { name: 'Today' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '7 Days' })).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'This week' })
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '30 Days' })).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Current month' })
    ).toBeInTheDocument()
  })

  test('opening the page displays and requests one hour before and after the current time', async () => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date('2026-11-04T12:30:45'))

    await renderPage()
    fireEvent.click(
      screen.getByRole('button', {
        name: '2026-11-04 11:30:45 ~ 2026-11-04 13:30:45',
      })
    )

    expect(screen.getByLabelText('Start Time')).toHaveValue(
      '2026-11-04T11:30:45.000'
    )
    expect(screen.getByLabelText('End Time')).toHaveValue(
      '2026-11-04T13:30:45.000'
    )
    await waitFor(() => {
      expect(listRequests).toHaveBeenCalledExactlyOnceWith({
        p: 1,
        page_size: 20,
        start_timestamp: new Date('2026-11-04T11:30:45').getTime() / 1000,
        end_timestamp: new Date('2026-11-04T13:30:45').getTime() / 1000,
      })
    })
  })

  test('repeated Query clicks with unchanged filters request and render the latest results', async () => {
    await renderPage()
    expect(await screen.findByText('Initial upstream failure')).toBeVisible()
    const queryButton = screen.getByRole('button', { name: 'Query' })
    await waitFor(() => expect(queryButton).toBeEnabled())
    fireEvent.click(queryButton)
    await waitFor(() => {
      expect(listRequests).toHaveBeenCalledTimes(2)
      expect(queryButton).toBeEnabled()
    })

    logMessage = 'Latest upstream failure'
    fireEvent.click(queryButton)

    expect(await screen.findByText('Latest upstream failure')).toBeVisible()
    expect(screen.queryByText('Initial upstream failure')).toBeNull()
    expect(listRequests).toHaveBeenCalledTimes(3)
    await waitFor(() => expect(queryButton).toBeEnabled())

    logMessage = ''
    fireEvent.click(queryButton)

    expect(
      await screen.findByText('No error logs for this range')
    ).toBeVisible()
    expect(screen.queryByText('Latest upstream failure')).toBeNull()
    expect(listRequests).toHaveBeenCalledTimes(4)
  })

  test('Query applies edited filters and refreshes previously cached conditions', async () => {
    await renderPage()
    expect(await screen.findByText('Initial upstream failure')).toBeVisible()
    const queryButton = screen.getByRole('button', { name: 'Query' })
    await waitFor(() => expect(queryButton).toBeEnabled())
    fireEvent.click(queryButton)
    await waitFor(() => {
      expect(listRequests).toHaveBeenCalledTimes(2)
      expect(queryButton).toBeEnabled()
    })

    fireEvent.change(screen.getByLabelText('Request ID'), {
      target: { value: ' req-latest ' },
    })
    expect(listRequests).toHaveBeenCalledTimes(2)
    logMessage = 'Filtered upstream failure'
    fireEvent.click(queryButton)
    expect(await screen.findByText('Filtered upstream failure')).toBeVisible()
    expect(listRequests).toHaveBeenLastCalledWith(
      expect.objectContaining({ p: 1, request_id: 'req-latest' })
    )
    await waitFor(() => expect(queryButton).toBeEnabled())

    fireEvent.change(screen.getByLabelText('Request ID'), {
      target: { value: '' },
    })
    logMessage = 'Refreshed unfiltered failure'
    fireEvent.click(queryButton)

    expect(
      await screen.findByText('Refreshed unfiltered failure')
    ).toBeVisible()
    expect(listRequests).toHaveBeenCalledTimes(4)
    expect(listRequests).toHaveBeenLastCalledWith(
      expect.not.objectContaining({ request_id: expect.anything() })
    )
  })

  test('Query from a later page refreshes the cached first page', async () => {
    await renderPage()
    expect(await screen.findByText('Initial upstream failure')).toBeVisible()
    const queryButton = screen.getByRole('button', { name: 'Query' })
    await waitFor(() => expect(queryButton).toBeEnabled())
    fireEvent.click(queryButton)
    await waitFor(() => {
      expect(listRequests).toHaveBeenCalledTimes(2)
      expect(queryButton).toBeEnabled()
    })
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    await waitFor(() => {
      expect(listRequests).toHaveBeenLastCalledWith(
        expect.objectContaining({ p: 2 })
      )
      expect(queryButton).toBeEnabled()
    })

    logMessage = 'Refreshed first page failure'
    fireEvent.click(queryButton)

    expect(
      await screen.findByText('Refreshed first page failure')
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
    expect(listRequests).toHaveBeenCalledTimes(4)
    expect(listRequests).toHaveBeenLastCalledWith(
      expect.objectContaining({ p: 1 })
    )
  })
})

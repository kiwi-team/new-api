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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { SectionPageLayout } from '@/components/layout'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { AlertsPage } from '../../index'
import { buildRule, ruleDefaults } from '../../lib/rule-form'
import type { AlertRule } from '../../types'
import { RulesList } from '../rules-list'

function fixture(
  options: { fail?: boolean; baseParams?: string; ruleName?: string } = {}
) {
  const request = vi
    .spyOn(api, 'request')
    .mockImplementation(async (config) => {
      if (options.fail) throw new Error('List unavailable')
      const params = new URL(config.url ?? '', 'http://localhost').searchParams
      const page = Number(params.get('page'))
      const filtered =
        params.get('q') || params.get('status') || params.get('metric')
      const id = filtered ? 1 : 251 - (page - 1) * 20
      const row = {
        id,
        owner_user_id: 1,
        config: buildRule(ruleDefaults()),
        revision: 1,
        effective_at: 0,
      } satisfies AlertRule
      row.config.name = options.ruleName ?? `Visible rule ${id}`
      return {
        data: {
          success: true,
          data: {
            items: [row],
            total: filtered ? 1 : 251,
            page,
            page_size: Number(params.get('limit')),
          },
        },
      }
    })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <SectionPageLayout>
        <SectionPageLayout.Title>Rules</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <RulesList
            baseParams={options.baseParams ?? 'owner_user_id=1'}
            scopes={['account', 'channel']}
            metrics={['success_rate', 'duration_ms']}
            canManage={() => true}
            onView={vi.fn()}
            onEdit={vi.fn()}
            onPause={vi.fn()}
            onDelete={vi.fn()}
          />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    </QueryClientProvider>
  )
  return request
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

it('loads server pages beyond 200 rules and resets the page when applying filters', async () => {
  const request = fixture({ baseParams: 'scope=all&channel_id=49' })
  const user = userEvent.setup()
  expect(await screen.findByText('Visible rule 251')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Go to last page' }))
  expect(await screen.findByText('Visible rule 11')).toBeInTheDocument()
  expect(request.mock.lastCall?.[0].url).toContain('page=13')
  expect(request.mock.lastCall?.[0].url).toContain('channel_id=49')
  await user.click(screen.getByRole('combobox', { name: '' }))
  await user.click(await screen.findByRole('option', { name: '50' }))
  await waitFor(() => {
    expect(request.mock.lastCall?.[0].url).toContain('limit=50')
    expect(request.mock.lastCall?.[0].url).toContain('page=1')
  })
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Status' }),
    'paused'
  )
  expect(await screen.findByText('Visible rule 1')).toBeInTheDocument()
  expect(request.mock.lastCall?.[0].url).toContain('page=1')
  expect(request.mock.lastCall?.[0].url).toContain('status=paused')
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Metric' }),
    'success_rate'
  )
  await waitFor(() =>
    expect(request.mock.lastCall?.[0].url).toContain('metric=success_rate')
  )
  await user.type(
    screen.getByRole('textbox', { name: 'Search by name or ID' }),
    'usapi'
  )
  await user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(request.mock.lastCall?.[0].url).toContain('q=usapi')
  )
  await user.click(screen.getByRole('button', { name: 'Clear filters' }))
  expect(await screen.findByText('Visible rule 251')).toBeInTheDocument()
  expect(screen.getByRole('combobox', { name: 'Status' })).toHaveValue('')
  expect(
    screen.getByRole('textbox', { name: 'Search by name or ID' })
  ).toHaveValue('')
  expect(request.mock.lastCall?.[0].url).toContain('scope=all')
})

it('shows an error with retry instead of claiming there are no rules', async () => {
  fixture({ fail: true })
  expect(
    await screen.findByRole('button', { name: /Retry/i })
  ).toBeInTheDocument()
  expect(screen.queryByText('No alert rules')).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Go to next page' })
  ).not.toBeInTheDocument()
})

function pageFixture(readAll: boolean, rowCount = 0) {
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'viewer', role: readAll ? 100 : 1 })
  const request = vi
    .spyOn(api, 'request')
    .mockImplementation(async (config) => {
      let data: unknown = []
      if (config.url?.endsWith('/catalog')) {
        data = {
          scopes: ['account', 'channel'],
          metrics: ['success_rate'],
          target_types: [],
          enabled: true,
          encryption_configured: true,
          read_all: readAll,
          manage_platform: readAll,
          read_engine_health: false,
          api_keys: [],
        }
      }
      if (config.url?.startsWith('/api/alerts/rules?')) {
        const items = Array.from({ length: rowCount }, (_, index) => {
          const config = buildRule(ruleDefaults())
          config.name = `Layout rule ${index + 1}`
          return {
            id: index + 1,
            owner_user_id: 1,
            config,
            revision: 1,
            effective_at: 0,
          }
        })
        data = {
          items,
          total: rowCount + (rowCount > 0 ? 1 : 0),
          page: 1,
          page_size: 20,
        }
      }
      return { data: { success: true, data } }
    })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <AlertsPage />
    </QueryClientProvider>
  )
  return request
}

it('shows all permitted accounts by default and allows switching to personal rules', async () => {
  const request = pageFixture(true)
  const user = userEvent.setup()
  expect(await screen.findByText('No alert rules')).toBeInTheDocument()
  const ruleRequests = () =>
    request.mock.calls
      .map(([config]) => config.url ?? '')
      .filter((url) => url.startsWith('/api/alerts/rules?'))
  expect(ruleRequests()[0]).toContain('scope=all')
  await user.click(screen.getByRole('button', { name: 'All accounts' }))
  await waitFor(() => expect(ruleRequests().at(-1)).not.toContain('scope=all'))
  expect(screen.getByRole('button', { name: 'My alerts' })).toBeInTheDocument()
})

it('keeps ordinary users in their personal scope and renders valid empty pagination', async () => {
  const request = pageFixture(false)
  expect(await screen.findByText('No alert rules')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Go to next page' })).toBeDisabled()
  expect(
    screen.getByRole('button', { name: 'Go to previous page' })
  ).toBeDisabled()
  expect(
    screen.queryByRole('button', { name: 'All accounts' })
  ).not.toBeInTheDocument()
  const urls = request.mock.calls.map(([config]) => config.url ?? '')
  expect(
    urls
      .filter((url) => url.startsWith('/api/alerts/rules?'))
      .every((url) => !url.includes('scope=all'))
  ).toBe(true)
})

it('keeps pagination outside the scrolling rules panel when a full page is loaded', async () => {
  pageFixture(true, 20)
  expect(await screen.findByText('Layout rule 20')).toBeInTheDocument()
  const panel = screen.getByRole('tabpanel', { name: 'Rules' })
  const pagination = screen.getByRole('group', { name: 'Page' })
  expect(panel).not.toContainElement(pagination)
  const scrollArea = panel.closest('.overflow-auto')
  expect(scrollArea).not.toBeNull()
  expect(scrollArea).not.toContainElement(pagination)
  expect(screen.getByRole('main')).toHaveClass(
    'min-h-0',
    'flex-1',
    'overflow-hidden'
  )
  expect(screen.getByRole('button', { name: 'Go to next page' })).toBeEnabled()
})

it('uses compact pagination on narrow screens without hiding previous and next actions', async () => {
  const matchMedia = window.matchMedia.bind(window)
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    ...matchMedia(query),
    matches: query === '(max-width: 640px)',
  }))
  fixture()
  expect(await screen.findByText('Visible rule 251')).toBeInTheDocument()
  expect(screen.getByRole('navigation', { name: 'Page' })).toBeInTheDocument()
  expect(screen.getByText('1 / 13')).toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Go to previous page' })
  ).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Go to next page' })).toBeEnabled()
})

it('replaces the rule footer when switching to another list tab', async () => {
  pageFixture(true)
  const user = userEvent.setup()
  expect(await screen.findByRole('group', { name: 'Page' })).toBeInTheDocument()
  await user.click(screen.getByRole('tab', { name: 'Notification targets' }))
  expect(
    await screen.findByRole('button', { name: 'Create notification target' })
  ).toBeInTheDocument()
  expect(screen.queryByRole('group', { name: 'Page' })).not.toBeInTheDocument()
  expect(screen.getAllByRole('navigation', { name: 'Page' })).toHaveLength(1)
})

it('wraps long rule names within their column so the full name remains readable', async () => {
  const name =
    'usapi-gemini / gemini-3.8-flash / model request duration p95 with additional descriptive text'
  fixture({ ruleName: name })
  expect(await screen.findByText(name)).toHaveClass(
    'whitespace-normal',
    'break-words'
  )
})

it('scrolls to the new table page after pagination without jumping on initial load', async () => {
  const scroll = vi.spyOn(HTMLElement.prototype, 'scrollIntoView')
  fixture()
  const user = userEvent.setup()
  expect(await screen.findByText('Visible rule 251')).toBeInTheDocument()
  expect(scroll).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Go to next page' }))
  expect(await screen.findByText('Visible rule 231')).toBeInTheDocument()
  await waitFor(() =>
    expect(scroll).toHaveBeenCalledWith({ block: 'start', behavior: 'instant' })
  )
})

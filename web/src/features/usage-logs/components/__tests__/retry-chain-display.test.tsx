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
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next, { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, test, vi } from 'vitest'

import en from '@/i18n/locales/en.json'

import type { UsageLog } from '../../data/schema'
import { useCommonLogsColumns } from '../columns/common-logs-columns'
import { RetryChainDisplay } from '../retry-chain-display'
import { UsageLogsProvider } from '../usage-logs-provider'

vi.mock('@lobehub/icons', () => ({}))

const routeLog: UsageLog = {
  id: 1,
  user_id: 1,
  created_at: 1,
  type: 2,
  content: '',
  username: 'root',
  token_name: 'token',
  model_name: 'gpt-test',
  quota: 1,
  prompt_tokens: 1,
  completion_tokens: 1,
  use_time: 2,
  is_stream: false,
  channel: 1577,
  channel_name: 'fallback',
  token_id: 1,
  group: 'default',
  ip: '',
  other: JSON.stringify({
    admin_info: {
      use_channel: [1578, 1577],
      use_channel_time: [343, 1200],
    },
  }),
  request_id: 'req-1',
  race_result: 'winner',
  upstream_request_id: '',
}

function ChannelRoutePreview() {
  const table = useReactTable({
    data: [routeLog],
    columns: useCommonLogsColumns(true, true),
    getCoreRowModel: getCoreRowModel(),
  })
  const cell = table
    .getRowModel()
    .rows[0].getAllCells()
    .find((item) => item.column.id === 'channel')
  if (!cell) throw new Error('The log must have a channel column')
  return flexRender(cell.column.columnDef.cell, cell.getContext())
}

describe('retry chain display', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      'Retry Chain': 'Retry Chain',
    })
  })

  test('shows the complete retry chain without requiring interaction', () => {
    render(<RetryChainDisplay chain='1578(343ms) → 1577' />)

    expect(screen.getByText('1578(343ms) → 1577')).toBeVisible()
    expect(
      screen.getByLabelText('Retry Chain: 1578(343ms) → 1577')
    ).toBeVisible()
  })

  test('shows attempt timings, named hover details, and the race winner', async () => {
    const user = userEvent.setup()
    const testI18n = createInstance()
    await testI18n.init({
      lng: 'en',
      resources: { en },
      interpolation: { escapeValue: false },
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    client.setQueryData(
      ['usage-logs', 'filter-options', 'channels'],
      [
        { value: '1578', label: 'primary (ID: 1578)', name: 'primary' },
        { value: '1577', label: 'fallback (ID: 1577)', name: 'fallback' },
      ],
      { updatedAt: Date.now() + 60_000 }
    )

    render(
      <I18nextProvider i18n={testI18n}>
        <QueryClientProvider client={client}>
          <UsageLogsProvider>
            <ChannelRoutePreview />
          </UsageLogsProvider>
        </QueryClientProvider>
      </I18nextProvider>
    )

    const chain = screen.getByLabelText(
      'Retry Chain: 1578(343ms) → 1577(1.20s)'
    )
    expect(chain).toBeVisible()
    expect(screen.getByLabelText('Race result: Winner')).toBeVisible()

    await user.hover(chain)
    expect(
      await screen.findByText(
        'Chain: primary #1578(343ms) → fallback #1577(1.20s)'
      )
    ).toBeVisible()
    client.clear()
  })
})

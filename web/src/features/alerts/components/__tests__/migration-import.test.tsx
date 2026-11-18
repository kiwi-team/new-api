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

import { api } from '@/lib/api'

import type { Catalog } from '../../types'
import { MigrationImport, type MigrationCandidate } from '../migration-import'
import { MigrationsPanel } from '../migrations-panel'

function fixture(count = 2, disabled = false) {
  const candidates: MigrationCandidate[] = Array.from(
    { length: count },
    (_, index) => ({
      kind: 'token_spend',
      object_id: String(index + 1),
      name: `Business key ${index + 1}`,
      owner_user_id: 1,
      threshold_usd: '100',
      configured: true,
    })
  )
  candidates.push({
    kind: 'user_notifications',
    object_id: '2',
    name: 'No destination',
    owner_user_id: 2,
    threshold_usd: '',
    configured: false,
  })
  const pending = vi.fn()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <MigrationImport
        candidates={candidates}
        disabled={disabled}
        onPendingChange={pending}
      />
    </QueryClientProvider>
  )
  return { candidates, pending, client }
}

function panelFixture(data: unknown, failed = false) {
  const request = vi.spyOn(api, 'request').mockResolvedValue({
    data: { success: !failed, data, message: failed ? 'List unavailable' : '' },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const catalog: Catalog = {
    scopes: [],
    metrics: [],
    target_types: [],
    api_keys: [],
    enabled: true,
    actions_enabled: false,
    encryption_configured: true,
    read_all: true,
    manage_platform: true,
    read_engine_health: true,
  }
  render(
    <QueryClientProvider client={client}>
      <MigrationsPanel catalog={catalog} />
    </QueryClientProvider>
  )
  return { request, client }
}

const legacyKey = {
  id: 11,
  name: 'Legacy key',
  owner_user_id: 1,
  threshold_usd: 100,
  configured: true,
}
const legacyAccount = {
  kind: 'user_spend',
  object_id: '2',
  name: 'Legacy account',
  owner_user_id: 2,
  threshold_usd: '1000',
  configured: true,
}
const migration = {
  id: 'migrated-account',
  kind: 'user_spend',
  object_id: '2',
  rule_id: 31,
  owner_user_id: 2,
  executor: 'legacy',
  status: 'observing',
  revision: 1,
  cutover_at: 0,
  checkpoint: '{}',
}

it.each([
  {
    name: 'legacy token list is null',
    data: {
      legacy_tokens: null,
      legacy_quota_items: [legacyAccount],
      migrations: [migration],
    },
    configured: 0,
    visible: ['Legacy account'],
    existing: true,
  },
  {
    name: 'quota candidate list is null',
    data: {
      legacy_tokens: [legacyKey],
      legacy_quota_items: null,
      migrations: [migration],
    },
    configured: 1,
    visible: ['Legacy key', '2'],
    existing: true,
  },
  {
    name: 'migration list is null',
    data: {
      legacy_tokens: [legacyKey],
      legacy_quota_items: [legacyAccount],
      migrations: null,
    },
    configured: 2,
    visible: ['Legacy key', 'Legacy account'],
    existing: false,
  },
  {
    name: 'all lists are null',
    data: { legacy_tokens: null, legacy_quota_items: null, migrations: null },
    configured: 0,
    visible: [],
    existing: false,
  },
  {
    name: 'list fields are absent',
    data: {},
    configured: 0,
    visible: [],
    existing: false,
  },
  {
    name: 'all lists are empty arrays',
    data: { legacy_tokens: [], legacy_quota_items: [], migrations: [] },
    configured: 0,
    visible: [],
    existing: false,
  },
])(
  'keeps the migration panel usable when $name',
  async ({ data, configured, visible, existing }) => {
    const { request, client } = panelFixture(data)
    const bulk = await screen.findByRole('button', {
      name: `Import configured alerts (${configured})`,
    })
    if (configured === 0) expect(bulk).toBeDisabled()
    else expect(bulk).toBeEnabled()
    for (const name of visible) expect(screen.getByText(name)).toBeVisible()
    if (existing) {
      expect(screen.getByRole('button', { name: 'Cut over' })).toBeEnabled()
    } else {
      expect(
        screen.queryByRole('button', { name: 'Cut over' })
      ).not.toBeInTheDocument()
    }
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({ method: 'GET', url: '/api/alerts/migrations' })
    )
    client.clear()
  }
)

it('shows migration request errors and can retry into an empty nullable response', async () => {
  const { request, client } = panelFixture(null, true)
  const retry = await screen.findByRole('button', { name: 'Retry' })
  expect(
    screen.queryByRole('button', { name: 'Import configured alerts (0)' })
  ).not.toBeInTheDocument()
  request.mockResolvedValue({
    data: {
      success: true,
      data: { legacy_tokens: null, legacy_quota_items: null, migrations: null },
    },
  })
  await userEvent.setup().click(retry)
  expect(
    await screen.findByRole('button', { name: 'Import configured alerts (0)' })
  ).toBeDisabled()
  client.clear()
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('requires confirmation, skips absent destinations and shows individual failures', async () => {
  const request = vi
    .spyOn(api, 'request')
    .mockImplementation(async (config) => {
      const items = (
        config.data as { items: { kind: string; object_id: string }[] }
      ).items
      return {
        data: {
          success: true,
          data: {
            items: items.map((item) => ({
              ...item,
              success: item.object_id !== '2',
              error:
                item.object_id === '2'
                  ? {
                      message_key:
                        'Notification destination is blocked by network access rules.',
                    }
                  : undefined,
            })),
          },
        },
      }
    })
  const { pending, client } = fixture()
  const invalidated = vi.spyOn(client, 'invalidateQueries')
  const user = userEvent.setup()
  await user.click(
    screen.getByRole('button', { name: 'Import configured alerts (2)' })
  )
  expect(request).not.toHaveBeenCalled()
  expect(
    screen.getByText(
      'Copy the listed alert rules, destinations and checkpoints into the alert center as observers. Legacy alerts continue sending until each migration is cut over.'
    )
  ).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Import as observer' }))
  expect(await screen.findByRole('status')).toHaveTextContent(
    '1 succeeded, 1 failed'
  )
  expect(screen.getByText('Business key 2')).toBeInTheDocument()
  expect(
    screen.getByText(
      'Notification destination is blocked by network access rules.'
    )
  ).toBeInTheDocument()
  expect(request).toHaveBeenCalledTimes(1)
  expect(request.mock.calls[0][0]).toMatchObject({
    url: '/api/alerts/migrations/batch',
    data: {
      items: [
        { kind: 'token_spend', object_id: '1' },
        { kind: 'token_spend', object_id: '2' },
      ],
    },
  })
  expect(pending.mock.calls.map((args) => args[0])).toEqual([true, false])
  expect(invalidated).toHaveBeenCalledWith({ queryKey: ['alerts'] })
})

it('imports more than ten candidates in bounded batches and locks repeat submission while pending', async () => {
  let finish: (value: unknown) => void = () => {}
  const request = vi
    .spyOn(api, 'request')
    .mockImplementation(async (config) => {
      if (request.mock.calls.length === 1) {
        await new Promise((resolve) => {
          finish = resolve
        })
      }
      const items = (
        config.data as { items: { kind: string; object_id: string }[] }
      ).items
      return {
        data: {
          success: true,
          data: { items: items.map((item) => ({ ...item, success: true })) },
        },
      }
    })
  fixture(11)
  const user = userEvent.setup()
  await user.click(
    screen.getByRole('button', { name: 'Import configured alerts (11)' })
  )
  await user.click(screen.getByRole('button', { name: 'Import as observer' }))
  expect(
    screen.getByRole('button', { name: 'Import as observer' })
  ).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  finish(undefined)
  await waitFor(() => expect(request).toHaveBeenCalledTimes(2))
  expect(await screen.findByRole('status')).toHaveTextContent(
    '11 succeeded, 0 failed'
  )
  expect(
    (request.mock.calls[0][0].data as { items: unknown[] }).items
  ).toHaveLength(10)
  expect(
    (request.mock.calls[1][0].data as { items: unknown[] }).items
  ).toHaveLength(1)
})

it('disables bulk import when encryption is unavailable', () => {
  fixture(2, true)
  expect(
    screen.getByRole('button', { name: 'Import configured alerts (2)' })
  ).toBeDisabled()
})

it('disables bulk import when every destination is missing', () => {
  fixture(0)
  expect(
    screen.getByRole('button', { name: 'Import configured alerts (0)' })
  ).toBeDisabled()
})

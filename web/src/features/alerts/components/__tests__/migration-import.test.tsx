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

import { MigrationImport, type MigrationCandidate } from '../migration-import'

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

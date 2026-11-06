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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { buildRule, ruleDefaults } from '../../lib/rule-form'
import type { AlertRule, Catalog } from '../../types'
import { RuleEditor } from '../rule-editor'

const catalog: Catalog = {
  scopes: ['account', 'api_key'],
  metrics: ['duration_ms', 'spend_net'],
  target_types: ['email'],
  enabled: true,
  actions_enabled: false,
  encryption_configured: true,
  read_all: false,
  manage_platform: false,
  read_engine_health: false,
  api_keys: [{ id: 11, name: 'Business key' }],
}
function fixture(legacy = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const config = buildRule({
    ...ruleDefaults(undefined, 1),
    name: 'Business latency',
    entities: ['11'],
    targets: ['7'],
  })
  if (legacy) {
    config.management_mode = 'platform_enforced'
    config.scope.type = 'account'
    config.scope.group_mode = 'aggregate'
    config.condition.metric = 'legacy_notice'
    config.condition.legacy_signal = 'user_notifications'
    config.condition.window.duration_seconds = 60
    config.trigger.mode = 'once'
    config.acknowledgement.required = false
  }
  const rule: AlertRule = {
    id: 5,
    owner_user_id: 1,
    revision: 3,
    effective_at: 1,
    config,
  }
  render(
    <QueryClientProvider client={client}>
      <RuleEditor
        rule={rule}
        catalog={catalog}
        owner={1}
        targets={[
          {
            id: 7,
            name: 'Operations',
            owner_user_id: 1,
            type: 'email',
            language: 'en',
            enabled: true,
            revision: 1,
            address_hint: 'o…@example.com',
            last_test_status: '',
          },
        ]}
        onClose={vi.fn()}
        onSaved={vi.fn()}
      />
    </QueryClientProvider>
  )
  return { rule, client }
}
afterEach(() => vi.restoreAllMocks())

it('requires a new preview after editing the rule and shows target names from preview evidence', async () => {
  const user = userEvent.setup()
  const request = vi
    .spyOn(api, 'request')
    .mockImplementation(async (config) => ({
      data: {
        success: true,
        data: {
          config: (config.data as { config: unknown }).config,
          verification_context: {
            rule_id: 5,
            owner_user_id: 1,
            expected_revision: 3,
            config_hash: 'hash',
            operation: 'save',
          },
          targets: [
            {
              target: {
                type: 'api_key',
                id: 11,
                name: 'Business key',
                model: '',
              },
              measurement: {
                value: '90000',
                samples: '20',
                status: 'complete',
                window_start: 0,
                window_end: 1,
                request_ids: [],
              },
            },
          ],
        },
      },
    }))
  const { client } = fixture()
  const save = screen.getByRole('button', { name: 'Save' })
  expect(save).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  await waitFor(() => expect(save).toBeEnabled())
  expect(screen.getByRole('cell', { name: 'Business key' })).toBeVisible()
  await user.type(screen.getByRole('textbox', { name: 'Name' }), ' updated')
  expect(save).toBeDisabled()
  expect(request).toHaveBeenCalledTimes(1)
  client.clear()
})

it('keeps save disabled when preview fails and does not show a successful preview', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'request').mockRejectedValue(new Error('preview unavailable'))
  const { client } = fixture()
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Preview' })).toBeEnabled()
  )
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  expect(
    screen.queryByRole('region', { name: 'Preview' })
  ).not.toBeInTheDocument()
  client.clear()
})

it('keeps migrated triggers read-only and permits a name edit to be previewed', async () => {
  const request = vi
    .spyOn(api, 'request')
    .mockImplementation(async (config) => ({
      data: {
        success: true,
        data: {
          config: (config.data as { config: unknown }).config,
          targets: [],
          verification_context: {
            rule_id: 5,
            owner_user_id: 1,
            expected_revision: 3,
            config_hash: 'hash',
            operation: 'save',
          },
        },
      },
    }))
  fixture(true)
  expect(screen.getByLabelText('Metric')).toBeDisabled()
  expect(screen.getByLabelText('Monitored object')).toBeDisabled()
  expect(screen.getByLabelText('Hold duration (seconds)')).toHaveAttribute(
    'readonly'
  )
  expect(
    screen.queryByRole('button', { name: 'Replay last hour' })
  ).not.toBeInTheDocument()
  expect(screen.getByRole('switch', { name: 'Enabled' })).toBeEnabled()
  await userEvent.clear(screen.getByLabelText('Name'))
  await userEvent.type(screen.getByLabelText('Name'), 'Renamed legacy notice')
  await userEvent.click(screen.getByRole('button', { name: 'Preview' }))
  await waitFor(() =>
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          config: expect.objectContaining({
            name: 'Renamed legacy notice',
            condition: expect.objectContaining({
              legacy_signal: 'user_notifications',
            }),
          }),
        }),
      })
    )
  )
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
})

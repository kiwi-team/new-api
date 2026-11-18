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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { buildRule, ruleDefaults } from '../../lib/rule-form'
import type { AlertRule, AlertTarget, Catalog, RuleSpec } from '../../types'
import { RuleEditor } from '../rule-editor'
import { PreviewResults } from '../rule-preview'
import { ReplayResults } from '../rule-replay'

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
const feishuTargets: AlertTarget[] = [
  {
    id: 6,
    name: 'Personal Feishu',
    owner_user_id: 1,
    type: 'feishu',
    enabled: true,
    revision: 1,
    language: 'en',
    address_hint: '',
    last_test_status: '',
  },
  {
    id: 16,
    name: 'Platform Feishu',
    owner_user_id: 0,
    type: 'feishu',
    enabled: true,
    revision: 1,
    language: 'en',
    address_hint: '',
    last_test_status: '',
  },
]
function fixture(
  legacy = false,
  options: {
    managePlatform?: boolean
    owner?: number
    config?: RuleSpec
    catalog?: Partial<Catalog>
    targets?: AlertTarget[]
    create?: boolean
    initialChannelID?: number
    remoteTargets?: AlertTarget[]
  } = {}
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const config =
    options.config ??
    buildRule({
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
    owner_user_id: options.owner ?? 1,
    revision: 3,
    effective_at: 1,
    config,
  }
  if (options.managePlatform) {
    vi.spyOn(api, 'request').mockImplementation(async (request) => {
      if (request.method === 'GET') {
        let data: unknown = []
        if (request.url?.includes('/catalog?')) data = catalog
        if (request.url?.endsWith('/channel-models')) {
          data = [{ id: 49, name: 'usapi-gemini', models: 'gemini-3.8-flash' }]
        }
        if (request.url?.includes('/targets?')) {
          const params = new URL(request.url, 'https://console.example')
            .searchParams
          const keyword = params.get('q')?.toLowerCase()
          const ids = params.get('ids')?.split(',').map(Number)
          const before = Number(
            params.get('before_id') || Number.MAX_SAFE_INTEGER
          )
          const all = [
            ...new Map(
              [
                ...feishuTargets,
                {
                  id: 7,
                  name: 'Operations',
                  owner_user_id: options.owner ?? 1,
                  type: 'email',
                  enabled: true,
                  revision: 1,
                  language: 'en',
                  address_hint: '',
                  last_test_status: '',
                },
                {
                  id: 8,
                  name: 'Remote operations',
                  owner_user_id: 245,
                  type: 'email',
                  enabled: true,
                  revision: 1,
                  language: 'en',
                  address_hint: '',
                  last_test_status: '',
                },
                ...(options.targets ?? []),
                ...(options.remoteTargets ?? []),
              ].map((target) => [target.id, target])
            ).values(),
          ]
          data = all
            .filter(
              (target) =>
                target.id < before &&
                (!ids || ids.includes(target.id)) &&
                (!keyword ||
                  target.name.toLowerCase().includes(keyword) ||
                  String(target.id) === keyword)
            )
            .sort((a, b) => b.id - a.id)
            .slice(0, Number(params.get('limit') || 200))
        }
        return {
          data: {
            success: true,
            data,
          },
        }
      }
      const input = request.data as {
        config: RuleSpec
        owner_user_id: number
      }
      if (request.url?.endsWith('/rules/backtest')) {
        return { data: { success: true, data: [] } }
      }
      return {
        data: {
          success: true,
          data: {
            config: input.config,
            targets: [],
            verification_context: {
              rule_id: rule.id,
              owner_user_id: input.owner_user_id,
              expected_revision: rule.revision,
              config_hash: 'hash',
              operation: 'save',
            },
          },
        },
      }
    })
  }
  const onSaved = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <RuleEditor
        rule={options.create ? undefined : rule}
        initialChannelID={options.initialChannelID}
        catalog={{
          ...catalog,
          ...options.catalog,
          manage_platform: options.managePlatform ?? false,
        }}
        owner={1}
        targets={
          options.targets ?? [
            {
              id: 7,
              name: 'Operations',
              owner_user_id: options.owner ?? 1,
              type: 'email',
              language: 'en',
              enabled: true,
              revision: 1,
              address_hint: 'o…@example.com',
              last_test_status: '',
            },
          ]
        }
        onClose={vi.fn()}
        onSaved={onSaved}
      />
    </QueryClientProvider>
  )
  return { rule, client, onSaved }
}
afterEach(() => vi.restoreAllMocks())

it('previews a renamed rule with its saved safety and delivery policies intact', async () => {
  const user = userEvent.setup()
  const config = buildRule({
    ...ruleDefaults(undefined, 1),
    name: 'Channel protection',
    scope: 'channel',
    entities: ['49'],
    targets: ['16'],
    action: 'disable_channel',
    autoRecovery: true,
    recoveryProtocol: 'openai',
    recoveryModels: ['gemini-3.8-flash'],
    recoverySuccesses: 5,
    recoveryObserveSeconds: 300,
    minRemainingChannels: 3,
    notifyRecovery: false,
    remindAfterSeconds: 60,
    escalateAfterSeconds: 120,
    reportBackfillPeriods: 2,
    reportPartialAfterSeconds: 3600,
  })
  const { client } = fixture(false, {
    config,
    owner: 0,
    managePlatform: true,
    targets: feishuTargets,
    catalog: {
      scopes: ['channel'],
      metrics: ['duration_ms'],
      actions_enabled: true,
    },
  })
  expect(await screen.findByText('gemini-3.8-flash')).toBeVisible()
  expect(
    screen.getByText(
      'At least 5 qualifying probes over 300 seconds are required. Small probes do not verify full production capacity.'
    )
  ).toBeVisible()
  await user.clear(screen.getByLabelText('Name'))
  await user.type(screen.getByLabelText('Name'), 'Renamed channel protection')
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
  expect(api.request).toHaveBeenCalledWith(
    expect.objectContaining({
      method: 'POST',
      url: '/api/alerts/rules/preview',
      data: expect.objectContaining({
        config: expect.objectContaining({
          name: 'Renamed channel protection',
          notify: config.notify,
          action_guard: config.action_guard,
          recovery: config.recovery,
          acknowledgement: config.acknowledgement,
          trigger: config.trigger,
        }),
      }),
    })
  )
  client.clear()
})

function mockOwnerDirectory(
  users = [
    { id: 1, username: 'operator', display_name: 'Current operator' },
    { id: 245, username: 'remote-customer', display_name: 'Remote Alice' },
  ]
) {
  return vi.spyOn(api, 'get').mockImplementation(async (path, config) => {
    const url = new URL(path, 'https://console.example')
    const match = url.pathname.match(/^\/api\/user\/(\d+)$/)
    if (match) {
      return {
        data: {
          success: true,
          data: users.find((user) => user.id === Number(match[1])),
        },
      }
    }
    const page = Number(config?.params?.p ?? url.searchParams.get('p') ?? 1)
    const pageSize = Number(
      config?.params?.page_size ?? url.searchParams.get('page_size') ?? 50
    )
    const keyword = (url.searchParams.get('keyword') ?? '').toLowerCase()
    const matches = keyword
      ? users.filter(
          (user) =>
            user.display_name.toLowerCase().includes(keyword) ||
            user.username.toLowerCase().includes(keyword) ||
            String(user.id).includes(keyword)
        )
      : users.filter((user) => user.id <= 200)
    return {
      data: {
        success: true,
        data: {
          items: matches.slice((page - 1) * pageSize, page * pageSize),
          total: keyword ? matches.length : Math.max(users.length, 245),
          page,
          page_size: pageSize,
        },
      },
    }
  })
}

it.each(['aLi', '24'])(
  'searches all owners by "%s" and previews the selected user beyond the first 200',
  async (keyword) => {
    const user = userEvent.setup()
    const get = mockOwnerDirectory()
    const { client } = fixture(false, { managePlatform: true })
    const owner = screen.getByRole('combobox', { name: 'Owner user' })
    await waitFor(() => expect(owner).toHaveTextContent('Current operator'))
    await user.click(owner)
    await screen.findByRole('option', { name: /Current operator/ })
    expect(screen.queryByRole('option', { name: /Remote Alice/ })).toBeNull()
    await user.type(
      screen.getByRole('combobox', { name: 'Filter by name or ID...' }),
      keyword
    )
    const option = await screen.findByRole('option', {
      name: /Remote Alice.*245/,
    })
    expect(get).toHaveBeenCalledWith(
      expect.stringContaining(`keyword=${keyword}`)
    )
    if (keyword === '24') await user.keyboard('{ArrowDown}{Enter}')
    else await user.click(option)
    expect(owner).toHaveTextContent('Remote Alice')
    expect(owner).toHaveTextContent('245')
    expect(screen.queryByText('Operations')).not.toBeInTheDocument()
    await user.click(screen.getByLabelText('Notification targets'))
    await user.click(
      await screen.findByRole('option', { name: /^Remote operations/ })
    )
    await user.keyboard('{Escape}')
    await user.click(screen.getByRole('button', { name: 'Preview' }))
    await waitFor(() =>
      expect(api.request).toHaveBeenCalledWith(
        expect.objectContaining({
          data: expect.objectContaining({
            owner_user_id: 245,
            config: expect.objectContaining({
              scope: expect.objectContaining({ entity_ids: [] }),
              notify: expect.objectContaining({ target_ids: [8] }),
            }),
          }),
        })
      )
    )
    client.clear()
  }
)

it('continues loading owner pages past 200 users', async () => {
  const user = userEvent.setup()
  const users = Array.from({ length: 205 }, (_, index) => ({
    id: index + 1,
    username: `member-${index + 1}`,
    display_name: `Customer ${index + 1}`,
  }))
  const get = mockOwnerDirectory(users)
  // All accounts remain available to pagination, including the last page.
  get.mockImplementation(async (path, config) => {
    if (path === '/api/user/1') {
      return { data: { success: true, data: users[0] } }
    }
    const page = Number(config?.params?.p ?? 1)
    const size = Number(config?.params?.page_size ?? 50)
    return {
      data: {
        success: true,
        data: {
          items: users.slice((page - 1) * size, page * size),
          total: users.length,
          page,
          page_size: size,
        },
      },
    }
  })
  const { client } = fixture(false, { managePlatform: true })
  await user.click(screen.getByRole('combobox', { name: 'Owner user' }))
  for (const page of [2, 3, 4, 5]) {
    const more = await screen.findByRole('button', { name: 'Load more users' })
    await waitFor(() => expect(more).toBeEnabled())
    await user.click(more)
    await waitFor(() =>
      expect(get).toHaveBeenCalledWith(
        '/api/user/',
        expect.objectContaining({
          params: expect.objectContaining({ p: page }),
        })
      )
    )
  }
  expect(
    await screen.findByRole('option', { name: /Customer 205.*205/ })
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Load more users' })
  ).not.toBeInTheDocument()
  client.clear()
})

it('lets Root create a rule for another owner with notification and escalation targets beyond the first 200', async () => {
  const user = userEvent.setup()
  mockOwnerDirectory()
  const remoteTargets = Array.from({ length: 205 }, (_, index) => ({
    ...feishuTargets[0],
    id: 1000 + index,
    owner_user_id: 2,
    name: index === 0 ? 'Finance alerts' : `Remote target ${index}`,
  }))
  const { client, onSaved } = fixture(false, {
    create: true,
    managePlatform: true,
    remoteTargets,
  })
  const owner = screen.getByRole('combobox', { name: 'Owner user' })
  expect(owner).toBeEnabled()
  await user.click(owner)
  await user.type(
    screen.getByRole('combobox', { name: 'Filter by name or ID...' }),
    'Alice'
  )
  await user.click(
    await screen.findByRole('option', { name: /Remote Alice.*245/ })
  )
  await user.type(screen.getByLabelText('Name'), 'Customer key alert')

  const notify = screen.getByLabelText('Notification targets')
  await user.click(notify)
  expect(
    screen.queryByRole('option', { name: /^Finance alerts/ })
  ).not.toBeInTheDocument()
  await user.click(
    await screen.findByRole('button', {
      name: 'Load more notification targets',
    })
  )
  expect(
    await screen.findByRole('option', { name: /^Finance alerts/ })
  ).toBeVisible()
  await user.type(notify, 'Finance')
  await waitFor(() =>
    expect(api.request).toHaveBeenCalledWith(
      expect.objectContaining({
        method: 'GET',
        url: '/api/alerts/targets?scope=all&limit=200&q=Finance',
      })
    )
  )
  await user.click(screen.getByRole('option', { name: /^Finance alerts/ }))
  await user.click(screen.getByLabelText('Name'))
  expect(
    screen.getByRole('switch', { name: 'Platform enforced' })
  ).toBeChecked()

  expect(screen.getByText('Finance alerts')).toBeVisible()

  const escalation = screen.getByLabelText('Escalation targets')
  await user.type(escalation, '1001')
  await waitFor(() =>
    expect(api.request).toHaveBeenCalledWith(
      expect.objectContaining({
        method: 'GET',
        url: '/api/alerts/targets?scope=all&limit=200&q=1001',
      })
    )
  )
  await user.click(
    await screen.findByRole('option', { name: 'Remote target 1' })
  )
  await user.click(screen.getByLabelText('Name'))
  expect(screen.getByText('Finance alerts')).toBeVisible()
  expect(screen.getByText('Remote target 1')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(onSaved).toHaveBeenCalled())
  for (const path of ['rules/preview', 'rules']) {
    expect(api.request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: `/api/alerts/${path}`,
        data: expect.objectContaining({
          owner_user_id: 245,
          config: expect.objectContaining({
            management_mode: 'platform_enforced',
            notify: expect.objectContaining({ target_ids: [1000] }),
            acknowledgement: expect.objectContaining({
              escalation_target_ids: [1001],
            }),
          }),
        }),
      })
    )
  }
  client.clear()
})

it('restores selected targets outside the first page and retries a failed target list', async () => {
  const user = userEvent.setup()
  mockOwnerDirectory()
  const config = buildRule({
    ...ruleDefaults(undefined, 1),
    name: 'Existing rule',
    entities: ['11'],
    targets: ['9'],
    escalationTargets: Array.from({ length: 21 }, (_, index) =>
      String(10 + index)
    ),
    enforced: true,
  })
  const remoteTargets = Array.from({ length: 205 }, (_, index) => ({
    ...feishuTargets[0],
    id: 1000 + index,
    name: `Paged target ${index}`,
  }))
  remoteTargets.push({
    ...feishuTargets[0],
    id: 9,
    name: 'Stored notification',
  })
  for (let id = 10; id <= 30; id++) {
    remoteTargets.push({
      ...feishuTargets[0],
      id,
      name: `Stored escalation ${id}`,
    })
  }
  const { client } = fixture(false, {
    config,
    managePlatform: true,
    remoteTargets,
  })
  expect(await screen.findByText('Stored notification')).toBeVisible()
  expect(await screen.findByText('Stored escalation 30')).toBeVisible()
  for (const [request] of vi.mocked(api.request).mock.calls) {
    if (!request.url?.includes('ids=')) continue
    const ids = new URL(request.url, 'https://console.example').searchParams
      .get('ids')
      ?.split(',')
    expect(ids?.length).toBeLessThanOrEqual(20)
  }
  const notify = screen.getByLabelText('Notification targets')
  await user.click(notify)
  vi.mocked(api.request).mockRejectedValueOnce(
    new Error('Target list unavailable')
  )
  await user.type(notify, 'missing-target')
  expect(
    await screen.findByText('Failed to load notification targets')
  ).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('No matching items')).toBeVisible()
  await user.keyboard('{Escape}')
  expect(screen.getByText('Stored notification')).toBeVisible()
  expect(screen.getByText('Stored escalation 30')).toBeVisible()
  client.clear()
})

it('keeps the owner selector and cross-account target requests unavailable to ordinary users', async () => {
  const request = vi.spyOn(api, 'request')
  const { client } = fixture(false, { create: true })
  expect(
    screen.queryByRole('combobox', { name: 'Owner user' })
  ).not.toBeInTheDocument()
  expect(request).not.toHaveBeenCalledWith(
    expect.objectContaining({
      method: 'GET',
      url: expect.stringContaining('scope=all'),
    })
  )
  client.clear()
})

it('keeps the selected owner name visible while the user detail request is pending', async () => {
  const user = userEvent.setup()
  const get = mockOwnerDirectory()
  const normalGet = get.getMockImplementation()
  if (!normalGet) throw new Error('Missing owner directory fixture')
  let resolveDetail!: (response: {
    data: { success: boolean; data: object }
  }) => void
  const detail = new Promise<{ data: { success: boolean; data: object } }>(
    (resolve) => {
      resolveDetail = resolve
    }
  )
  get.mockImplementation((path, config) =>
    path === '/api/user/245' ? detail : normalGet(path, config)
  )
  const { client } = fixture(false, { managePlatform: true })
  const owner = screen.getByRole('combobox', { name: 'Owner user' })
  await user.click(owner)
  await user.type(
    screen.getByRole('combobox', { name: 'Filter by name or ID...' }),
    'Alice'
  )
  await user.click(await screen.findByRole('option', { name: /Remote Alice/ }))
  expect(owner).toHaveTextContent('Remote Alice')
  expect(owner).toHaveAttribute('aria-expanded', 'false')
  resolveDetail({
    data: {
      success: true,
      data: {
        id: 245,
        username: 'remote-customer',
        display_name: 'Remote Alice',
      },
    },
  })
  await waitFor(() => expect(owner).toHaveTextContent('Remote Alice'))
  client.clear()
})

it.each([false, true])(
  'loads the existing owner name outside the first page and preserves legacy read-only state (%s)',
  async (legacy) => {
    const get = mockOwnerDirectory()
    const { client } = fixture(legacy, {
      managePlatform: true,
      owner: 245,
    })
    const owner = screen.getByRole('combobox', { name: 'Owner user' })
    await waitFor(() => expect(owner).toHaveTextContent('Remote Alice'))
    expect(get).toHaveBeenCalledWith('/api/user/245')
    expect(owner).toHaveTextContent('245')
    if (legacy) expect(owner).toBeDisabled()
    else expect(owner).toBeEnabled()
    client.clear()
  }
)

it('retries a failed owner search and shows an empty result without accepting arbitrary IDs', async () => {
  const user = userEvent.setup()
  const get = mockOwnerDirectory()
  const { client } = fixture(false, { managePlatform: true })
  await user.click(screen.getByRole('combobox', { name: 'Owner user' }))
  await screen.findByRole('option', { name: /Current operator/ })
  get.mockRejectedValueOnce(new Error('user search unavailable'))
  await user.type(
    screen.getByRole('combobox', { name: 'Filter by name or ID...' }),
    'missing'
  )
  expect(await screen.findByText('Failed to load users')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  expect(await screen.findByText('No matching items')).toBeVisible()
  await user.keyboard('{Enter}{Escape}')
  expect(
    screen.getByRole('combobox', { name: 'Owner user' })
  ).toHaveTextContent('Current operator')
  client.clear()
})

it('keeps the comparison controls adjacent and previews the condition with its calculation settings', async () => {
  const user = userEvent.setup()
  const request = vi
    .spyOn(api, 'request')
    .mockImplementation(async (input) => ({
      data: {
        success: true,
        data: {
          config: (input.data as { config: RuleSpec }).config,
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
  const { client } = fixture()
  const condition = within(
    screen.getByRole('group', { name: 'Alert condition' })
  )
  const calculation = within(
    screen.getByRole('group', { name: 'Calculation settings' })
  )
  const metric = condition.getByLabelText('Metric')
  const operator = condition.getByLabelText('Comparison operator')
  const threshold = condition.getByLabelText(
    'Threshold (milliseconds or count)'
  )
  await user.click(metric)
  await user.tab()
  expect(operator).toHaveFocus()
  await user.tab()
  expect(threshold).toHaveFocus()
  expect(condition.queryByLabelText('Severity')).not.toBeInTheDocument()

  await user.selectOptions(operator, 'gte')
  await user.clear(threshold)
  await user.type(threshold, '45000')
  await user.clear(condition.getByLabelText('Recovery threshold'))
  await user.type(condition.getByLabelText('Recovery threshold'), '15000')
  await user.selectOptions(calculation.getByLabelText('Aggregation'), 'p99')
  await user.clear(calculation.getByLabelText('Window (minutes)'))
  await user.type(calculation.getByLabelText('Window (minutes)'), '10')
  await user.clear(calculation.getByLabelText('Minimum valid samples'))
  await user.type(calculation.getByLabelText('Minimum valid samples'), '50')
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
  expect(request).toHaveBeenCalledWith(
    expect.objectContaining({
      data: expect.objectContaining({
        config: expect.objectContaining({
          condition: expect.objectContaining({
            metric: 'duration_ms',
            operator: 'gte',
            threshold_ms: 45000,
            recovery_threshold_ms: 15000,
            aggregation: 'p99',
            min_samples: 50,
            window: expect.objectContaining({
              type: 'rolling',
              duration_seconds: 600,
            }),
          }),
        }),
      }),
    })
  )
  client.clear()
})

it('edits saved channel success rates as percentages and submits integer basis points', async () => {
  const user = userEvent.setup()
  const config = buildRule({
    ...ruleDefaults(undefined, 1),
    name: 'usapi-gemini success rate',
    targets: ['7'],
    scope: 'channel',
    metric: 'success_rate',
    operator: 'lt',
    entities: ['49'],
  })
  config.condition.threshold_bps = 8000
  config.condition.recovery_threshold_bps = 9900
  const { client } = fixture(false, {
    config,
    owner: 0,
    managePlatform: true,
    catalog: { scopes: ['channel'], metrics: ['success_rate'] },
  })
  const condition = within(
    screen.getByRole('group', { name: 'Alert condition' })
  )
  const threshold = condition.getByLabelText('Threshold (%)')
  const recovery = condition.getByLabelText('Recovery threshold (%)')
  expect(threshold).toHaveValue(80)
  expect(recovery).toHaveValue(99)
  expect(threshold).toHaveAttribute('min', '0')
  expect(threshold).toHaveAttribute('max', '100')
  expect(threshold).toHaveAttribute('step', '0.01')
  expect(condition.getAllByText('%')).toHaveLength(2)
  expect(screen.queryByLabelText('Aggregation')).not.toBeInTheDocument()
  await user.clear(threshold)
  await user.type(threshold, '80.25')
  await user.clear(recovery)
  await user.type(recovery, '99.99')
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
  expect(api.request).toHaveBeenCalledWith(
    expect.objectContaining({
      method: 'POST',
      data: expect.objectContaining({
        config: expect.objectContaining({
          condition: expect.objectContaining({
            threshold_bps: 8025,
            recovery_threshold_bps: 9999,
          }),
        }),
      }),
    })
  )
  client.clear()
})

it('updates percentage units and hides unused aggregation when the metric changes', async () => {
  const user = userEvent.setup()
  const { client } = fixture(false, {
    catalog: {
      metrics: [
        'duration_ms',
        'ttft_ms',
        'success_rate',
        'failure_rate',
        'request_count',
        'spend_net',
      ],
    },
  })
  const metric = screen.getByLabelText('Metric')
  expect(screen.getByLabelText('Aggregation')).toBeVisible()
  for (const rate of ['success_rate', 'failure_rate']) {
    await user.selectOptions(metric, rate)
    expect(screen.getByLabelText('Threshold (%)')).toBeVisible()
    expect(screen.getByLabelText('Recovery threshold (%)')).toBeVisible()
    expect(screen.queryByLabelText('Aggregation')).not.toBeInTheDocument()
  }
  for (const count of ['request_count', 'spend_net']) {
    await user.selectOptions(metric, count)
    expect(screen.queryByText('%')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Threshold (%)')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Aggregation')).not.toBeInTheDocument()
  }
  await user.selectOptions(metric, 'ttft_ms')
  expect(screen.getByLabelText('Aggregation')).toBeVisible()
  expect(screen.queryByText('%')).not.toBeInTheDocument()
  client.clear()
})

it('blocks out-of-range percentages with field errors and allows a corrected preview', async () => {
  const user = userEvent.setup()
  const { client } = fixture(false, {
    managePlatform: true,
    catalog: { metrics: ['duration_ms', 'success_rate'] },
  })
  await user.selectOptions(screen.getByLabelText('Metric'), 'success_rate')
  const threshold = screen.getByLabelText('Threshold (%)')
  const recovery = screen.getByLabelText('Recovery threshold (%)')
  await user.clear(threshold)
  await user.type(threshold, '101')
  await user.clear(recovery)
  await user.type(recovery, '100.01')
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  await waitFor(() => expect(threshold).toHaveAttribute('aria-invalid', 'true'))
  expect(recovery).toHaveAttribute('aria-invalid', 'true')
  expect(threshold).toHaveAccessibleDescription(
    'Enter a percentage from 0 to 100 with up to two decimal places.'
  )
  expect(api.request).not.toHaveBeenCalledWith(
    expect.objectContaining({ method: 'POST' })
  )
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  await user.clear(threshold)
  await user.type(threshold, '80')
  await user.clear(recovery)
  await user.type(recovery, '99.99')
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
  expect(threshold).toHaveAttribute('aria-invalid', 'false')
  client.clear()
})

it('explains overlapping success and recovery thresholds before previewing and accepts recovery at 100%', async () => {
  const user = userEvent.setup()
  const { client } = fixture(false, {
    managePlatform: true,
    catalog: { metrics: ['duration_ms', 'success_rate'] },
  })
  await user.selectOptions(screen.getByLabelText('Metric'), 'success_rate')
  const threshold = screen.getByLabelText('Threshold (%)')
  const recovery = screen.getByLabelText('Recovery threshold (%)')
  await user.clear(threshold)
  await user.type(threshold, '100')
  expect(recovery).toHaveValue(99)
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  await waitFor(() => expect(recovery).toHaveAttribute('aria-invalid', 'true'))
  expect(recovery).toHaveAccessibleDescription(
    'Recovery threshold must be at least the alert threshold.'
  )
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  expect(api.request).not.toHaveBeenCalledWith(
    expect.objectContaining({ method: 'POST' })
  )
  await user.clear(recovery)
  await user.type(recovery, '100')
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
  expect(api.request).toHaveBeenCalledWith(
    expect.objectContaining({
      method: 'POST',
      data: expect.objectContaining({
        config: expect.objectContaining({
          condition: expect.objectContaining({
            operator: 'lt',
            threshold_bps: 10000,
            recovery_threshold_bps: 10000,
          }),
        }),
      }),
    })
  )
  client.clear()
})

it('requires a destination for a notification-only channel rule and previews after choosing one', async () => {
  const user = userEvent.setup()
  const config = buildRule({
    ...ruleDefaults(undefined, 1),
    name: 'usapi-gemini channel',
    scope: 'channel',
    metric: 'success_rate',
    operator: 'lt',
    threshold: '95',
    recoveryThreshold: '99',
    entities: ['49'],
    targets: [],
  })
  const { client } = fixture(false, {
    config,
    owner: 0,
    managePlatform: true,
    catalog: { scopes: ['channel'], metrics: ['success_rate'] },
  })
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  const targets = screen.getByLabelText('Notification targets')
  await waitFor(() => expect(targets).toHaveAttribute('aria-invalid', 'true'))
  expect(targets).toHaveAccessibleDescription(
    'Select at least one notification target or automatic action.'
  )
  expect(api.request).not.toHaveBeenCalledWith(
    expect.objectContaining({ method: 'POST' })
  )
  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  await user.click(targets)
  await user.click(await screen.findByRole('option', { name: /^Operations/ }))
  await user.keyboard('{Escape}')
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
  expect(api.request).toHaveBeenCalledWith(
    expect.objectContaining({
      method: 'POST',
      data: expect.objectContaining({
        owner_user_id: 0,
        config: expect.objectContaining({
          notify: expect.objectContaining({ target_ids: [7] }),
          actions: [],
          condition: expect.objectContaining({
            threshold_bps: 9500,
            recovery_threshold_bps: 9900,
          }),
        }),
      }),
    })
  )
  expect(targets).toHaveAttribute('aria-invalid', 'false')
  client.clear()
})

it.each([
  'channel',
  'channel_model',
  'system',
  'uid_budget',
  'project_budget',
] as const)(
  'offers all users’ targets after switching to %s and clears previous selections',
  async (scope) => {
    const user = userEvent.setup()
    const config = buildRule({
      ...ruleDefaults(undefined, 1),
      name: 'Switch scope',
      targets: ['6'],
      escalationTargets: ['6'],
    })
    const { client } = fixture(false, {
      config,
      managePlatform: true,
      catalog: { scopes: ['api_key', scope] },
      targets: feishuTargets,
    })
    expect(screen.getAllByText('Personal Feishu')).toHaveLength(2)
    await user.selectOptions(screen.getByLabelText('Monitored object'), scope)
    expect(screen.queryByText('Personal Feishu')).not.toBeInTheDocument()
    for (const label of ['Notification targets', 'Escalation targets']) {
      await user.click(screen.getByLabelText(label))
      expect(
        await screen.findByRole('option', { name: /^Platform Feishu/ })
      ).toBeVisible()
      expect(
        screen.getByRole('option', { name: /^Personal Feishu/ })
      ).toBeVisible()
      await user.keyboard('{Escape}')
    }
    client.clear()
  }
)

it('creates a channel rule with platform targets consistently in replay, preview and save', async () => {
  const user = userEvent.setup()
  const { client, onSaved } = fixture(false, {
    create: true,
    managePlatform: true,
    catalog: {
      scopes: ['api_key', 'channel'],
      metrics: ['duration_ms', 'success_rate'],
    },
    targets: feishuTargets,
  })
  await user.type(screen.getByLabelText('Name'), 'usapi-gemini')
  await user.selectOptions(screen.getByLabelText('Monitored object'), 'channel')
  await user.selectOptions(screen.getByLabelText('Metric'), 'success_rate')
  await user.click(screen.getByLabelText('Channels'))
  await user.click(await screen.findByRole('option', { name: 'usapi-gemini' }))
  await user.keyboard('{Escape}')
  for (const label of ['Notification targets', 'Escalation targets']) {
    await user.click(screen.getByLabelText(label))
    expect(
      screen.getByRole('option', { name: /^Personal Feishu/ })
    ).toBeVisible()
    await user.click(
      await screen.findByRole('option', { name: /^Platform Feishu/ })
    )
    await user.keyboard('{Escape}')
  }
  await user.click(screen.getByRole('button', { name: 'Replay last hour' }))
  await waitFor(() =>
    expect(api.request).toHaveBeenCalledWith(
      expect.objectContaining({ url: '/api/alerts/rules/backtest' })
    )
  )
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(onSaved).toHaveBeenCalledWith(0))
  for (const path of ['rules/backtest', 'rules/preview', 'rules']) {
    expect(api.request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: `/api/alerts/${path}`,
        data: expect.objectContaining({
          owner_user_id: 0,
          config: expect.objectContaining({
            scope: expect.objectContaining({
              type: 'channel',
              channel_ids: [49],
            }),
            notify: expect.objectContaining({ target_ids: [16] }),
            acknowledgement: expect.objectContaining({
              escalation_target_ids: [16],
            }),
          }),
        }),
      })
    )
  }
  client.clear()
})

it('blocks disabled destinations in a platform rule before replay or preview', async () => {
  const user = userEvent.setup()
  const config = buildRule({
    ...ruleDefaults(undefined, 1),
    name: 'Invalid selection',
    scope: 'channel',
    entities: ['49'],
    targets: ['6'],
    escalationTargets: ['6'],
  })
  const { client } = fixture(false, {
    config,
    owner: 0,
    managePlatform: true,
    catalog: { scopes: ['channel'] },
    targets: [
      {
        id: 6,
        name: 'Personal Feishu',
        owner_user_id: 1,
        type: 'feishu',
        enabled: false,
        revision: 1,
        language: 'en',
        address_hint: '',
        last_test_status: '',
      },
    ],
  })
  await user.click(screen.getByRole('button', { name: 'Replay last hour' }))
  await waitFor(() =>
    expect(screen.getByLabelText('Notification targets')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
  )
  expect(
    screen.getByLabelText('Escalation targets')
  ).toHaveAccessibleDescription(
    'Enable the selected notification target before enabling this rule.'
  )
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  expect(api.request).not.toHaveBeenCalledWith(
    expect.objectContaining({ method: 'POST' })
  )
  client.clear()
})

it('restores the owner selector when Root switches back to a custom rule and previews the chosen owner', async () => {
  const user = userEvent.setup()
  mockOwnerDirectory()
  const { client } = fixture(false, {
    create: true,
    managePlatform: true,
    catalog: {
      scopes: ['api_key', 'account', 'channel_model'],
      metrics: ['duration_ms', 'failure_rate', 'spend_net'],
    },
  })
  const template = screen.getByLabelText('Rule template')
  expect(screen.getByRole('combobox', { name: 'Owner user' })).toBeEnabled()
  for (const preset of ['Channel model failures', 'Daily account summary']) {
    await user.selectOptions(template, preset)
    if (preset === 'Channel model failures') {
      expect(
        screen.queryByRole('combobox', { name: 'Owner user' })
      ).not.toBeInTheDocument()
    }
    await user.selectOptions(template, '')
    expect(screen.getByRole('combobox', { name: 'Owner user' })).toBeEnabled()
    expect(screen.getByLabelText('Monitored object')).toHaveValue('api_key')
    expect(screen.getByLabelText('Metric')).toHaveValue('duration_ms')
    expect(screen.getByLabelText('Name')).toHaveValue('')
  }
  const owner = screen.getByRole('combobox', { name: 'Owner user' })
  await user.click(owner)
  await user.type(
    screen.getByRole('combobox', { name: 'Filter by name or ID...' }),
    '245'
  )
  await user.click(
    await screen.findByRole('option', { name: /Remote Alice.*245/ })
  )
  await user.type(screen.getByLabelText('Name'), 'Custom customer latency')
  await user.click(screen.getByLabelText('API Keys'))
  await user.click(await screen.findByRole('option', { name: 'Business key' }))
  await user.click(screen.getByLabelText('Name'))
  await user.click(screen.getByLabelText('Notification targets'))
  await user.click(
    await screen.findByRole('option', { name: 'Remote operations' })
  )
  await user.click(screen.getByLabelText('Name'))
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  )
  expect(api.request).toHaveBeenCalledWith(
    expect.objectContaining({
      url: '/api/alerts/rules/preview',
      data: expect.objectContaining({
        owner_user_id: 245,
        config: expect.objectContaining({
          name: 'Custom customer latency',
          scope: expect.objectContaining({ type: 'api_key', entity_ids: [11] }),
          condition: expect.objectContaining({ metric: 'duration_ms' }),
          notify: expect.objectContaining({ target_ids: [8] }),
        }),
      }),
    })
  )
  client.clear()
})

it('restores personal defaults when an ordinary user leaves a spending template for a custom rule', async () => {
  const user = userEvent.setup()
  const { client } = fixture(false, { create: true })
  const template = screen.getByLabelText('Rule template')
  await user.selectOptions(template, 'API key spending milestones')
  expect(screen.getByLabelText('Metric')).toHaveValue('spend_net')
  await user.selectOptions(template, '')
  expect(screen.getByLabelText('Metric')).toHaveValue('duration_ms')
  expect(screen.getByLabelText('Name')).toHaveValue('')
  expect(
    screen.queryByRole('combobox', { name: 'Owner user' })
  ).not.toBeInTheDocument()
  client.clear()
})

it.each(['template', 'channel-link'])(
  'offers all users’ targets when creating from %s',
  async (entry) => {
    const user = userEvent.setup()
    const { client } = fixture(false, {
      create: true,
      initialChannelID: entry === 'channel-link' ? 49 : undefined,
      managePlatform: true,
      catalog: {
        scopes: ['api_key', 'channel_model'],
        metrics: ['duration_ms', 'failure_rate'],
      },
      targets: feishuTargets,
    })
    if (entry === 'template') {
      await user.selectOptions(
        screen.getByLabelText('Rule template'),
        'Channel model failures'
      )
    }
    await user.click(screen.getByLabelText('Notification targets'))
    expect(
      await screen.findByRole('option', { name: /^Platform Feishu/ })
    ).toBeVisible()
    expect(
      screen.getByRole('option', { name: /^Personal Feishu/ })
    ).toBeVisible()
    client.clear()
  }
)

it('preserves platform recipients for root-enforced personal rules and clears them when enforcement is removed', async () => {
  const user = userEvent.setup()
  const config = buildRule({
    ...ruleDefaults(undefined, 1),
    name: 'Enforced key',
    enforced: true,
    targets: ['16'],
    escalationTargets: ['16'],
  })
  const { client } = fixture(false, {
    config,
    managePlatform: true,
    targets: feishuTargets,
  })
  expect(screen.getAllByText('Platform Feishu')).toHaveLength(2)
  await user.click(screen.getByRole('switch', { name: 'Platform enforced' }))
  expect(screen.queryByText('Platform Feishu')).not.toBeInTheDocument()
  await user.click(screen.getByLabelText('Notification targets'))
  expect(
    await screen.findByRole('option', { name: /^Personal Feishu/ })
  ).toBeVisible()
  expect(screen.getByRole('option', { name: /^Platform Feishu/ })).toBeVisible()
  client.clear()
})

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

it.each([
  {
    name: 'compatible user notifications',
    scope: 'account' as const,
    trigger: 'once',
    legacy: true,
  },
  {
    name: 'account daily report',
    scope: 'account' as const,
    trigger: 'schedule_report',
    legacy: false,
  },
  {
    name: 'account spending',
    scope: 'account' as const,
    trigger: 'milestone',
    legacy: false,
  },
  {
    name: 'API key spending',
    scope: 'api_key' as const,
    trigger: 'milestone',
    legacy: false,
  },
])(
  'edits, previews and saves $name with null collections from the API',
  async (input) => {
    const user = userEvent.setup()
    const request = vi
      .spyOn(api, 'request')
      .mockImplementation(async (request) => {
        const config = (request.data as { config: RuleSpec }).config
        if (request.method === 'PATCH') {
          return {
            data: {
              success: true,
              data: {
                id: 5,
                owner_user_id: 1,
                revision: 4,
                effective_at: 1,
                config,
              },
            },
          }
        }
        return {
          data: {
            success: true,
            data: {
              config: { ...config, actions: null },
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
        }
      })
    const config = buildRule({
      ...ruleDefaults(undefined, 1),
      name: input.name,
      scope: input.scope,
      metric: input.legacy ? 'legacy_notice' : 'spend_net',
      legacySignal: input.legacy ? 'user_notifications' : undefined,
      trigger: input.trigger,
      window: input.trigger === 'schedule_report' ? 'day' : 'cumulative',
      operator: 'gte',
      threshold: input.legacy ? '1' : '0',
      recoveryThreshold: '0',
      hold: 0,
      entities: input.scope === 'api_key' ? ['11'] : [],
      targets: ['7'],
    })
    config.actions = null
    config.scope.channel_ids = null
    if (input.scope === 'account') delete config.scope.entity_ids
    config.scope.model_selector.models = null
    config.acknowledgement.escalation_target_ids = null
    const { client, onSaved } = fixture(input.legacy, { config })
    expect(screen.getByRole('textbox', { name: 'Name' })).toHaveValue(
      input.name
    )
    if (input.legacy) expect(screen.getByLabelText('Metric')).toBeDisabled()
    await user.clear(screen.getByRole('textbox', { name: 'Name' }))
    await user.type(
      screen.getByRole('textbox', { name: 'Name' }),
      `${input.name} updated`
    )
    await user.click(screen.getByRole('button', { name: 'Preview' }))
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
    )
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          config: expect.objectContaining({
            name: `${input.name} updated`,
            scope: expect.objectContaining({
              type: input.scope,
              entity_ids: input.scope === 'api_key' ? [11] : [],
            }),
            trigger: expect.objectContaining({ mode: input.trigger }),
            notify: expect.objectContaining({ target_ids: [7] }),
            actions: [],
            acknowledgement: expect.objectContaining({
              escalation_target_ids: [],
            }),
          }),
        }),
      })
    )
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith(1))
    client.clear()
  }
)

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

it('configures a model traffic baseline, accepts growth over 100%, and validates history before preview', async () => {
  const user = userEvent.setup()
  const { client } = fixture(false, {
    managePlatform: true,
    owner: 0,
    targets: feishuTargets,
    catalog: {
      scopes: ['channel_model', 'model'],
      metrics: ['request_started_count', 'request_volume_change'],
    },
    config: buildRule({
      ...ruleDefaults(undefined, 1),
      name: 'Model traffic surge',
      scope: 'model',
      metric: 'request_volume_change',
      models: ['gemini-3.8-flash'],
      baselineMode: 'manual',
      baselineCount: 200,
      changeDirection: 'increase',
      threshold: '150',
      recoveryThreshold: '20',
      targets: ['16'],
    }),
  })
  expect(screen.getByLabelText('Change threshold (%)')).toHaveValue(150)
  expect(screen.getByLabelText('Baseline calls per window')).toHaveValue(200)
  expect(screen.queryByLabelText('Aggregation')).not.toBeInTheDocument()
  expect(
    screen.queryByLabelText('Minimum valid samples')
  ).not.toBeInTheDocument()
  await user.selectOptions(
    screen.getByLabelText('Comparison baseline'),
    'historical'
  )
  expect(
    screen.queryByLabelText('Baseline calls per window')
  ).not.toBeInTheDocument()
  const history = screen.getByLabelText('Baseline history (minutes)')
  await user.clear(history)
  await user.type(history, '4')
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  expect(
    await screen.findByText(
      'Baseline history must be at least the statistics window.'
    )
  ).toBeVisible()
  expect(history).toHaveAttribute('aria-invalid', 'true')
  expect(
    vi
      .mocked(api.request)
      .mock.calls.some(([request]) => request.url?.endsWith('/rules/preview'))
  ).toBe(false)
  await user.clear(history)
  await user.type(history, '30')
  await user.click(screen.getByRole('button', { name: 'Preview' }))
  await waitFor(() =>
    expect(api.request).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          owner_user_id: 0,
          config: expect.objectContaining({
            scope: expect.objectContaining({ type: 'model' }),
            condition: expect.objectContaining({
              metric: 'request_volume_change',
              change_direction: 'increase',
              threshold_bps: 15000,
              min_samples: 0,
              baseline: {
                mode: 'historical',
                history_seconds: 1800,
                minimum_count: 20,
              },
            }),
            actions: [],
          }),
        }),
      })
    )
  )
  client.clear()
})

it('shows percentages in preview and replay and withholds values while a baseline is unavailable', () => {
  const config = buildRule({
    ...ruleDefaults(undefined, 1),
    name: 'Traffic drop',
    metric: 'request_volume_change',
    threshold: '50',
    recoveryThreshold: '20',
  })
  const target = {
    type: 'model',
    id: 0,
    name: 'gemini-3.8-flash',
    model: 'gemini-3.8-flash',
  }
  const measurement = {
    value: '6000',
    samples: '80',
    numerator: '0',
    missing: '0',
    status: 'complete',
    window_start: 1700000000000,
    window_end: 1700000300000,
    request_ids: [],
  }
  const warming = {
    ...measurement,
    status: 'warming_up',
    reason: 'baseline_insufficient',
  }
  render(
    <>
      <PreviewResults
        preview={{
          config,
          verification_context: {
            rule_id: 1,
            owner_user_id: 0,
            expected_revision: 1,
            config_hash: 'fixture',
            operation: 'save',
          },
          targets: [
            { target, measurement },
            { target: { ...target, id: 1 }, measurement: warming },
          ],
        }}
      />
      <ReplayResults
        metric='request_volume_change'
        points={[
          { target, at: 1700000300000, measurement, matched: true },
          { target, at: 1700000360000, measurement: warming, matched: false },
        ]}
      />
    </>
  )
  expect(screen.getAllByRole('cell', { name: '60%' })).toHaveLength(2)
  expect(screen.getAllByRole('cell', { name: '—' })).toHaveLength(2)
  expect(screen.queryByRole('cell', { name: '6000' })).not.toBeInTheDocument()
})

it.each([
  ['success_rate', '0', '—'],
  ['request_volume_change', '10000', '100%'],
])(
  'uses valid denominators for %s in preview and replay',
  (metric, value, expected) => {
    const config = buildRule({ ...ruleDefaults(undefined, 1), metric })
    const target = {
      type: 'channel',
      id: 49,
      name: 'Example channel',
      model: '',
    }
    const measurement = {
      value,
      samples: '0',
      numerator: '0',
      missing: '0',
      status: 'complete',
      window_start: 1,
      window_end: 2,
      request_ids: [],
    }
    render(
      <>
        <PreviewResults
          preview={{
            config,
            verification_context: {
              rule_id: 1,
              owner_user_id: 0,
              expected_revision: 1,
              config_hash: 'fixture',
              operation: 'save',
            },
            targets: [{ target, measurement }],
          }}
        />
        <ReplayResults
          metric={metric}
          points={[{ target, at: 2, measurement, matched: false }]}
        />
      </>
    )
    expect(screen.getAllByRole('cell', { name: expected })).toHaveLength(2)
  }
)

it('opens timing help without submitting the form and supports Escape and keyboard reopening', async () => {
  const user = userEvent.setup()
  const { client } = fixture(false, {
    managePlatform: true,
    owner: 0,
    targets: feishuTargets,
    catalog: { scopes: ['channel_model'], metrics: ['request_volume_change'] },
    config: buildRule({
      ...ruleDefaults(undefined, 1),
      name: 'Traffic surge',
      scope: 'channel_model',
      entities: ['49'],
      models: ['gemini-3.8-flash'],
      targets: ['16'],
      metric: 'request_volume_change',
      baselineMode: 'manual',
      baselineCount: 2,
      changeDirection: 'increase',
      threshold: '100',
      recoveryThreshold: '20',
      minutes: 1,
      hold: 15,
      recoveryHold: 20,
    }),
  })
  const hold = screen.getByLabelText('Hold duration (seconds)')
  const trigger = screen.getByRole('button', {
    name: 'Learn more: Hold duration (seconds)',
  })
  await user.click(trigger)
  const help = await screen.findByRole('dialog', {
    name: 'Hold duration (seconds)',
  })
  expect(trigger).toHaveAttribute('aria-expanded', 'true')
  expect(
    within(help).getByText(/a single burst may leave the window too soon/)
  ).toBeVisible()
  expect(hold).toHaveValue(15)
  await user.keyboard('{Escape}')
  await waitFor(() => expect(trigger).toHaveFocus())
  expect(trigger).toHaveAttribute('aria-expanded', 'false')
  expect(
    screen.queryByRole('dialog', { name: 'Hold duration (seconds)' })
  ).not.toBeInTheDocument()
  await user.keyboard('{Enter}')
  expect(
    await screen.findByRole('dialog', { name: 'Hold duration (seconds)' })
  ).toBeVisible()
  await user.keyboard('{Escape}')
  await user.click(
    screen.getByRole('button', { name: 'Learn more: Recovery hold (seconds)' })
  )
  const recovery = await screen.findByRole('dialog', {
    name: 'Recovery hold (seconds)',
  })
  expect(
    within(recovery).getByText(
      /Set 0 to resolve on the next matching evaluation/
    )
  ).toBeVisible()
  expect(
    screen.getByRole('spinbutton', { name: 'Recovery hold (seconds)' })
  ).toHaveValue(20)
  expect(
    vi
      .mocked(api.request)
      .mock.calls.some(([request]) => request.method === 'POST')
  ).toBe(false)
  client.clear()
})

it('explains recovery against the frozen baseline for each direction and historical normalization', async () => {
  const user = userEvent.setup()
  const { client } = fixture(false, {
    managePlatform: true,
    owner: 0,
    targets: feishuTargets,
    catalog: { scopes: ['channel_model'], metrics: ['request_volume_change'] },
    config: buildRule({
      ...ruleDefaults(undefined, 1),
      name: 'Traffic change',
      scope: 'channel_model',
      entities: ['49'],
      models: ['gemini-3.8-flash'],
      targets: ['16'],
      metric: 'request_volume_change',
      baselineMode: 'historical',
      changeDirection: 'increase',
      threshold: '50',
      recoveryThreshold: '20',
    }),
  })
  const trigger = screen.getByRole('button', {
    name: 'Learn more: Recovery change threshold (%)',
  })
  await user.click(trigger)
  let help = await screen.findByRole('dialog', {
    name: 'Recovery change threshold (%)',
  })
  expect(within(help).getByText(/baseline frozen/)).toBeVisible()
  expect(within(help).getByText(/240 calls or fewer/)).toBeVisible()
  await user.keyboard('{Escape}')
  await user.selectOptions(
    screen.getByLabelText('Change direction'),
    'decrease'
  )
  await user.click(trigger)
  help = await screen.findByRole('dialog', {
    name: 'Recovery change threshold (%)',
  })
  expect(within(help).getByText(/160 calls or more/)).toBeVisible()
  expect(within(help).queryByText(/240 calls or fewer/)).not.toBeInTheDocument()
  await user.keyboard('{Escape}')
  await user.click(
    screen.getByRole('button', {
      name: 'Learn more: Baseline history (minutes)',
    })
  )
  const history = await screen.findByRole('dialog', {
    name: 'Baseline history (minutes)',
  })
  expect(
    within(history).getByText(
      /120 historical calls become a baseline of 20 calls per 5 minutes/
    )
  ).toBeVisible()
  await user.click(screen.getByLabelText('Name'))
  await waitFor(() =>
    expect(
      screen.queryByRole('dialog', { name: 'Baseline history (minutes)' })
    ).not.toBeInTheDocument()
  )
  client.clear()
})

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
import {
  createRouter,
  createRootRoute,
  createMemoryHistory,
  RouterContextProvider,
} from '@tanstack/react-router'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { api } = await import('@/lib/api')
const { ROLE } = await import('@/lib/roles')
const { useAuthStore } = await import('@/stores/auth-store')
const { ApiKeysProvider } = await import('../api-keys-provider')
const { ApiKeysMutateDrawer } = await import('../api-keys-mutate-drawer')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

type ApiMethod = (url: string, data?: unknown) => Promise<{ data: unknown }>
type MockableApi = {
  get: ApiMethod
  post: ApiMethod
}
type RenderedDrawer = {
  queryClient: InstanceType<typeof QueryClient>
}

const apiClient = api as unknown as MockableApi
const originalGet = apiClient.get
const originalPost = apiClient.post
let renderedDrawer: RenderedDrawer | null = null

type DirectoryUser = { id: number; username: string; display_name?: string }

function installApiFixtures(
  createdPayloads: Array<Record<string, unknown>>,
  users: DirectoryUser[] = [
    { id: 1, username: 'root' },
    { id: 42, username: 'alice' },
  ]
) {
  apiClient.get = async (url, config) => {
    const path = new URL(url, 'http://localhost')
    const userMatch = path.pathname.match(/^\/api\/user\/(\d+)$/)
    if (userMatch) {
      return {
        data: {
          success: true,
          data: users.find((user) => user.id === Number(userMatch[1])),
        },
      }
    }
    if (
      path.pathname === '/api/user/' ||
      path.pathname === '/api/user/search'
    ) {
      const params = (config as { params?: { p?: number; page_size?: number } })
        ?.params
      const page = Number(params?.p ?? path.searchParams.get('p') ?? 1)
      const pageSize = Math.min(
        100,
        Number(params?.page_size ?? path.searchParams.get('page_size') ?? 50)
      )
      const keyword = (path.searchParams.get('keyword') ?? '').toLowerCase()
      const matches = users.filter((user) =>
        [user.username, user.display_name ?? '', String(user.id)].some(
          (value) => value.toLowerCase().includes(keyword)
        )
      )
      return {
        data: {
          success: true,
          data: {
            items: matches.slice((page - 1) * pageSize, page * pageSize),
            total: matches.length,
            page,
            page_size: pageSize,
          },
        },
      }
    }
    switch (url) {
      case '/api/status':
        return { data: { data: { default_use_auto_group: true } } }
      case '/api/user/models':
        return { data: { success: true, data: [] } }
      case '/api/user/self/groups':
        return {
          data: {
            success: true,
            data: {
              auto: { desc: 'Automatic routing', ratio: 'auto' },
              default: { desc: 'Standard access', ratio: 1 },
              vip: { desc: 'Priority access', ratio: 2 },
            },
          },
        }
      case '/api/token/auto-groups':
        return {
          data: {
            success: true,
            data: { groups: ['vip', 'default'], max_count: 3 },
          },
        }
      default:
        throw new Error(`Unexpected GET ${url}`)
    }
  }
  apiClient.post = async (url, data) => {
    expect(url).toBe('/api/token/')
    expect(data && typeof data === 'object').toBeTruthy()
    createdPayloads.push(data as Record<string, unknown>)
    return { data: { success: true, data: {} } }
  }
}

async function renderCreateDrawer(): Promise<void> {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const freshAt = Date.now() + 60_000
  queryClient.setQueryData(
    ['status'],
    { default_use_auto_group: true },
    { updatedAt: freshAt }
  )
  queryClient.setQueryData(
    ['user-models'],
    { success: true, data: [] },
    { updatedAt: freshAt }
  )
  queryClient.setQueryData(
    ['user-groups'],
    {
      success: true,
      data: {
        auto: { desc: 'Automatic routing', ratio: 'auto' },
        default: { desc: 'Standard access', ratio: 1 },
        vip: { desc: 'Priority access', ratio: 2 },
      },
    },
    { updatedAt: freshAt }
  )
  queryClient.setQueryData(
    ['token-auto-groups'],
    {
      success: true,
      data: { groups: ['vip', 'default'], max_count: 3 },
    },
    { updatedAt: freshAt }
  )
  renderedDrawer = { queryClient }
  const router = createRouter({
    routeTree: createRootRoute(),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })

  render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <RouterContextProvider router={router}>
          <ApiKeysProvider>
            <ApiKeysMutateDrawer open onOpenChange={() => undefined} />
          </ApiKeysProvider>
        </RouterContextProvider>
      </I18nextProvider>
    </QueryClientProvider>
  )
  await waitFor(
    () => {
      const saveButton = findButton('Save changes', false)
      expect(saveButton).toBeEnabled()
    },
    { timeout: 1500 }
  )
}

function findButton(text: string, required: true): HTMLButtonElement
function findButton(text: string, required: false): HTMLButtonElement | null
function findButton(text: string, required = true): HTMLButtonElement | null {
  const button = screen
    .queryAllByRole<HTMLButtonElement>('button')
    .find((candidate) => candidate.textContent?.includes(text))
  if (required && !button) {
    throw new Error(`Expected button containing "${text}"`)
  }
  return button ?? null
}

function getControlByLabel(labelText: 'Name' | 'Quantity'): HTMLInputElement
function getControlByLabel(labelText: 'Group'): HTMLButtonElement
function getControlByLabel(labelText: 'Auto group order'): HTMLElement
function getControlByLabel(labelText: string): HTMLElement {
  const label = [...document.querySelectorAll<HTMLLabelElement>('label')].find(
    (candidate) => candidate.textContent?.trim() === labelText
  )
  if (!label) {
    throw new Error(`Expected label "${labelText}"`)
  }

  const control =
    label.control ??
    label
      .closest('[data-slot="form-item"]')
      ?.querySelector<HTMLElement>(
        '[data-slot="form-control"], input, textarea, button[role="combobox"], [role="group"]'
      )
  if (!control) {
    throw new Error(`Expected control for label "${labelText}"`)
  }
  return control
}

function changeInput(input: HTMLInputElement, value: string): void {
  fireEvent.input(input, { target: { value } })
}

function selectComboboxOption(
  trigger: HTMLButtonElement,
  optionDescription: string
): void {
  fireEvent.click(trigger)
  const option = [
    ...document.querySelectorAll<HTMLElement>('[data-slot="command-item"]'),
  ].find((candidate) => candidate.textContent?.includes(optionDescription))
  if (!option) {
    throw new Error(`Expected option containing "${optionDescription}"`)
  }
  fireEvent.click(option)
}

afterEach(() => {
  cleanup()
  apiClient.get = originalGet
  apiClient.post = originalPost
  useAuthStore.getState().auth.reset()
  localStorage.clear()
  if (renderedDrawer) {
    renderedDrawer.queryClient.clear()
    renderedDrawer = null
  }
})

describe('API keys mutate drawer Auto group integration', () => {
  test('inherits the root Auto order and sends an empty override for every batch-created key', async () => {
    const createdPayloads: Array<Record<string, unknown>> = []
    installApiFixtures(createdPayloads)
    await renderCreateDrawer()

    const groupTrigger = getControlByLabel('Group')
    expect(groupTrigger.textContent?.includes('auto')).toBe(true)
    expect(
      document.body.textContent?.includes(
        'Using the complete global Auto order (2 groups)'
      )
    ).toBe(true)
    expect(
      [
        ...document.querySelectorAll('[data-slot="global-auto-order-name"]'),
      ].map((item) => item.textContent)
    ).toEqual(['vip', 'default'])
    expect(findButton('Restore global Auto', true).disabled).toBe(true)

    changeInput(getControlByLabel('Name'), 'batch')
    changeInput(getControlByLabel('Quantity'), '2')
    fireEvent.click(findButton('Save changes', true))
    await waitFor(() => expect(createdPayloads).toHaveLength(2))

    expect(createdPayloads.length).toBe(2)
    expect(createdPayloads[0]?.name).toBe('batch')
    for (const payload of createdPayloads) {
      expect(payload.group).toBe('auto')
      expect(payload.auto_groups).toEqual([])
      expect(payload.cross_group_retry).toBe(true)
      expect(payload.user_id).toBeUndefined()
    }
    expect(document.body.textContent?.includes('Create for user')).toBe(false)
  })

  test('preserves an unsaved custom order and mode after Auto to ordinary to Auto changes', async () => {
    const createdPayloads: Array<Record<string, unknown>> = []
    installApiFixtures(createdPayloads)
    await renderCreateDrawer()

    const autoOrderControl = getControlByLabel('Auto group order')
    const addGroupTrigger = autoOrderControl.querySelector<HTMLButtonElement>(
      'button[role="combobox"]'
    )
    if (!addGroupTrigger) {
      throw new Error('Expected Auto group order combobox')
    }
    selectComboboxOption(addGroupTrigger, 'Priority access')

    expect(
      document.querySelector('button[aria-label="Remove vip"]')
    ).toBeTruthy()
    expect(document.body.textContent?.includes('1 / 3 groups selected')).toBe(
      true
    )
    expect(findButton('Restore global Auto', true).disabled).toBe(false)

    const groupTrigger = getControlByLabel('Group')
    selectComboboxOption(groupTrigger, 'Standard access')
    expect(document.querySelector('button[aria-label="Remove vip"]')).toBe(null)
    selectComboboxOption(groupTrigger, 'Automatic routing')

    expect(
      document.querySelector('button[aria-label="Remove vip"]')
    ).toBeTruthy()
    expect(document.body.textContent?.includes('1 / 3 groups selected')).toBe(
      true
    )
    expect(findButton('Restore global Auto', true).disabled).toBe(false)

    changeInput(getControlByLabel('Name'), 'custom')
    fireEvent.click(findButton('Save changes', true))
    await waitFor(() => expect(createdPayloads).toHaveLength(1))
    expect(createdPayloads[0]?.auto_groups).toEqual(['vip'])
  })

  test('lets root bind channel routing rules when creating a key', async () => {
    const createdPayloads: Array<Record<string, unknown>> = []
    installApiFixtures(createdPayloads)
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'root',
      role: ROLE.SUPER_ADMIN,
    })
    await renderCreateDrawer()

    fireEvent.click(findButton('Advanced Settings', true))
    fireEvent.change(screen.getByLabelText('Channel ratios'), {
      target: { value: '{"12":0.8}' },
    })
    fireEvent.change(screen.getByLabelText('Channel rules'), {
      target: { value: '{"gpt-4o":{"retry":2}}' },
    })
    expect(
      screen.queryByRole('switch', { name: 'Prioritize key channel rules' })
    ).not.toBeInTheDocument()
    expect(
      screen.getByText(
        /Routing priority: large files, key rules, global model routes, then system defaults\./
      )
    ).toBeVisible()
    expect(findButton('Visual editor', true)).toBeVisible()

    changeInput(getControlByLabel('Name'), 'root-routed')
    fireEvent.click(findButton('Save changes', true))
    await waitFor(() => expect(createdPayloads).toHaveLength(1))
    expect(createdPayloads[0]).toMatchObject({
      channel_ratios: '{"12":0.8}',
      channel_rules: '{"gpt-4o":{"retry":2}}',
      channel_rules_high_priority: false,
    })
  })

  test('lets root choose another key owner and submits that user ID', async () => {
    const createdPayloads: Array<Record<string, unknown>> = []
    installApiFixtures(createdPayloads)
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'root',
      role: ROLE.SUPER_ADMIN,
    })
    await renderCreateDrawer()

    const ownerControl = screen.getByRole('combobox', {
      name: 'Create for user',
    })
    const ownerTrigger = ownerControl
      .closest('[data-slot="form-item"]')
      ?.querySelector<HTMLButtonElement>('button')
    if (!ownerTrigger) throw new Error('Expected owner combobox trigger')
    fireEvent.click(ownerTrigger)
    fireEvent.click(
      await screen.findByRole('option', { name: 'alice (ID: 42)' })
    )

    changeInput(getControlByLabel('Name'), 'delegated')
    fireEvent.click(findButton('Save changes', true))
    await waitFor(() => expect(createdPayloads).toHaveLength(1))
    expect(createdPayloads[0]?.user_id).toBe(42)
  })
})

describe('API keys mutate drawer owner search', () => {
  test('keeps My account available when loading users fails and retries the directory', async () => {
    installApiFixtures([])
    const fixtureGet = apiClient.get
    let unavailable = true
    apiClient.get = async (url, config) => {
      if (url === '/api/user/' && unavailable) {
        return { data: { success: false, message: 'Directory unavailable' } }
      }
      return fixtureGet(url, config)
    }
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'root',
      role: ROLE.SUPER_ADMIN,
    })
    await renderCreateDrawer()

    const owner = screen.getByRole('combobox', { name: 'Create for user' })
    fireEvent.click(owner)
    expect(await screen.findByText('Failed to load users')).toBeVisible()
    expect(screen.getByRole('option', { name: 'My account' })).toBeVisible()
    expect(owner).toHaveTextContent('My account')
    unavailable = false
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(
      await screen.findByRole('option', { name: 'alice (ID: 42)' })
    ).toBeVisible()
  })

  test.each(['aLi', '20'])(
    'finds users beyond the first 200 by name or partial ID (%s) and submits their ID',
    async (keyword) => {
      const createdPayloads: Array<Record<string, unknown>> = []
      const users = Array.from({ length: 205 }, (_, index) => ({
        id: index + 1,
        username: index === 204 ? 'long-tail-user' : `member-${index + 1}`,
        display_name: index === 204 ? 'Remote Alice' : '',
      }))
      installApiFixtures(createdPayloads, users)
      useAuthStore.getState().auth.setUser({
        id: 1,
        username: 'root',
        role: ROLE.SUPER_ADMIN,
      })
      await renderCreateDrawer()

      const owner = screen.getByRole('combobox', { name: 'Create for user' })
      expect(owner).toHaveTextContent('My account')
      fireEvent.click(owner)
      expect(
        await screen.findByRole('option', { name: 'My account' })
      ).toBeVisible()
      expect(
        screen.queryByRole('option', { name: /Remote Alice/ })
      ).not.toBeInTheDocument()
      fireEvent.change(screen.getByPlaceholderText('Filter by name or ID...'), {
        target: { value: keyword },
      })
      fireEvent.click(
        await screen.findByRole('option', { name: /Remote Alice/ })
      )
      expect(owner).toHaveTextContent('Remote Alice (long-tail-user) (ID: 205)')
      changeInput(getControlByLabel('Name'), 'remote-owner')
      fireEvent.click(findButton('Save changes', true))
      await waitFor(() => expect(createdPayloads).toHaveLength(1))
      expect(createdPayloads[0]?.user_id).toBe(205)
    }
  )

  test('loads users after page 200 and can switch back to My account after a search', async () => {
    const createdPayloads: Array<Record<string, unknown>> = []
    installApiFixtures(
      createdPayloads,
      Array.from({ length: 205 }, (_, index) => ({
        id: index + 1,
        username: `member-${index + 1}`,
      }))
    )
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'root',
      role: ROLE.SUPER_ADMIN,
    })
    await renderCreateDrawer()

    const owner = screen.getByRole('combobox', { name: 'Create for user' })
    fireEvent.click(owner)
    for (const lastID of [50, 100, 150, 200]) {
      await screen.findByRole('option', {
        name: `member-${lastID} (ID: ${lastID})`,
      })
      fireEvent.click(screen.getByRole('button', { name: 'Load more users' }))
    }
    fireEvent.click(
      await screen.findByRole('option', { name: 'member-205 (ID: 205)' })
    )
    expect(owner).toHaveTextContent('member-205 (ID: 205)')

    fireEvent.click(owner)
    const search = screen.getByPlaceholderText('Filter by name or ID...')
    fireEvent.change(search, { target: { value: 'missing-user' } })
    expect(await screen.findByText('No matching items')).toBeVisible()
    expect(owner).toHaveTextContent('member-205 (ID: 205)')
    fireEvent.change(search, { target: { value: '' } })
    fireEvent.click(await screen.findByRole('option', { name: 'My account' }))
    expect(owner).toHaveTextContent('My account')

    changeInput(getControlByLabel('Name'), 'self-owner')
    fireEvent.click(findButton('Save changes', true))
    await waitFor(() => expect(createdPayloads).toHaveLength(1))
    expect(createdPayloads[0]?.user_id).toBeUndefined()
  })
})

describe('API keys mutate drawer user quota query switch', () => {
  test('sends false for a new key after the switch, which starts on, is turned off', async () => {
    const createdPayloads: Array<Record<string, unknown>> = []
    installApiFixtures(createdPayloads)
    await renderCreateDrawer()

    fireEvent.click(findButton('Advanced Settings', true))
    const quotaQuerySwitch = await screen.findByRole('switch', {
      name: 'Allow querying user quota',
    })
    expect(quotaQuerySwitch).toBeChecked()
    fireEvent.click(quotaQuerySwitch)
    expect(quotaQuerySwitch).not.toBeChecked()

    changeInput(getControlByLabel('Name'), 'no-user-quota')
    fireEvent.click(findButton('Save changes', true))
    await waitFor(() => expect(createdPayloads).toHaveLength(1))
    expect(createdPayloads[0]?.allow_user_quota_query).toBe(false)
  })
})

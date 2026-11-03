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
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  test,
  vi,
} from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { User } from '../../types'
import { UsersMutateDrawer } from '../users-mutate-drawer'
import { UsersProvider } from '../users-provider'

const target: User = {
  id: 7,
  username: 'managed-user',
  display_name: 'Managed',
  role: ROLE.USER,
  status: 1,
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'default',
  setting: '{}',
}

const clients: QueryClient[] = []
const originalGetAnimations = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

beforeAll(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})

afterAll(() => {
  if (originalGetAnimations) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      originalGetAnimations
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
})

beforeEach(() => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    switch (url) {
      case '/api/user/7':
        return {
          data: {
            success: true,
            data: {
              ...target,
              setting:
                '{"model_limits_enabled":true,"model_limits":["custom-existing"]}',
            },
          },
        }
      case '/api/group/':
        return { data: { success: true, data: ['default'] } }
      case '/api/authz/catalog':
        return { data: { success: true, data: { resources: [], roles: [] } } }
      case '/api/orgs/':
        return { data: { success: true, data: [] } }
      case '/api/channel/models_enabled':
        return { data: { success: true, data: ['gpt-4o'] } }
      default:
        throw new Error(`Unexpected request: ${url}`)
    }
  })
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  window.localStorage.clear()
})

function renderEditor(role: number = ROLE.SUPER_ADMIN) {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'operator', role })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  clients.push(client)
  const onOpenChange = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <UsersProvider>
        <UsersMutateDrawer
          open
          onOpenChange={onOpenChange}
          currentRow={target}
        />
      </UsersProvider>
    </QueryClientProvider>
  )
  return { user: userEvent.setup(), onOpenChange }
}

describe('root user model limits', () => {
  test('loads the target settings and saves selected and custom models', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const { user, onOpenChange } = renderEditor()

    const toggle = await screen.findByRole('switch', {
      name: 'Enable model limits',
    })
    await waitFor(() => expect(toggle).toBeChecked())
    expect(screen.getByText('custom-existing')).toBeVisible()

    const models = screen.getByRole('combobox', { name: 'Select models' })
    await user.click(models)
    await user.click(await screen.findByRole('option', { name: 'gpt-4o' }))
    await user.click(models)
    await user.paste('custom-new')
    await user.keyboard('{Enter}')
    expect(onOpenChange).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(put).toHaveBeenCalledWith(
      '/api/user/',
      expect.objectContaining({
        id: target.id,
        model_limits_enabled: true,
        model_limits: ['custom-existing', 'gpt-4o', 'custom-new'],
      }),
      {}
    )
  })

  test('disabling the switch hides the list and preserves models for re-enabling', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const { user, onOpenChange } = renderEditor()
    const toggle = await screen.findByRole('switch', {
      name: 'Enable model limits',
    })
    await waitFor(() => expect(toggle).toBeChecked())

    toggle.focus()
    await user.keyboard(' ')
    expect(toggle).not.toBeChecked()
    expect(
      screen.queryByRole('combobox', { name: 'Select models' })
    ).not.toBeInTheDocument()
    await user.click(toggle)
    expect(screen.getByText('custom-existing')).toBeVisible()
    await user.click(toggle)
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(put).toHaveBeenCalledWith(
      '/api/user/',
      expect.objectContaining({
        model_limits_enabled: false,
        model_limits: ['custom-existing'],
      }),
      {}
    )
  })

  test('ordinary administrators cannot see or submit model limit controls', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const { user, onOpenChange } = renderEditor(ROLE.ADMIN)
    await waitFor(() =>
      expect(screen.getByLabelText('Display Name')).toHaveValue('Managed')
    )
    expect(
      screen.queryByRole('switch', { name: 'Enable model limits' })
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    const payload: unknown = put.mock.calls[0]?.[1]
    expect(payload).not.toHaveProperty('model_limits_enabled')
    expect(payload).not.toHaveProperty('model_limits')
  })

  test('removing all selected models submits an empty allowlist', async () => {
    const put = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const { user, onOpenChange } = renderEditor()
    const toggle = await screen.findByRole('switch', {
      name: 'Enable model limits',
    })
    await waitFor(() => expect(toggle).toBeChecked())
    await user.click(screen.getByRole('combobox', { name: 'Select models' }))
    await user.click(
      await screen.findByRole('option', { name: 'custom-existing' })
    )
    await user.keyboard('{Escape}')
    expect(onOpenChange).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(put).toHaveBeenCalledWith(
      '/api/user/',
      expect.objectContaining({ model_limits: [] }),
      {}
    )
  })

  test('a failed save keeps the edited settings open for retry', async () => {
    vi.spyOn(api, 'put').mockResolvedValue({
      data: { success: false, message: 'Update failed' },
    })
    const { user, onOpenChange } = renderEditor()
    const toggle = await screen.findByRole('switch', {
      name: 'Enable model limits',
    })
    await waitFor(() => expect(toggle).toBeChecked())
    await user.click(toggle)
    await user.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Save changes' })).toBeEnabled()
    )
    expect(onOpenChange).not.toHaveBeenCalled()
    expect(toggle).not.toBeChecked()
  })
})

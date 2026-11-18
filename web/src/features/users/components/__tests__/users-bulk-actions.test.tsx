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
  type RowSelectionState,
} from '@tanstack/react-table'
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { userSchema, type User } from '../../types'
import { DataTableBulkActions } from '../data-table-bulk-actions'
import { useUsersColumns } from '../users-columns'
import { UsersProvider } from '../users-provider'

const users = [
  { id: 7, username: 'alice', email: 'alice@example.com', role: ROLE.USER },
  { id: 8, username: 'bob', email: 'bob@example.com', role: ROLE.ADMIN },
  { id: 1, username: 'root', email: '', role: ROLE.SUPER_ADMIN },
].map((user) =>
  userSchema.parse({
    ...user,
    display_name: user.username,
    quota: 0,
    used_quota: 0,
    request_count: 0,
    group: 'default',
    status: 1,
  })
)
const clients: QueryClient[] = []

function UsersListing(props: { users: User[] }) {
  const [rowSelection, onRowSelectionChange] = useState<RowSelectionState>({})
  const columns = useUsersColumns().filter(
    (column) =>
      column.id === 'select' ||
      ('accessorKey' in column &&
        ['username', 'email'].includes(String(column.accessorKey)))
  )
  const table = useReactTable({
    data: props.users,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getRowId: (user) => String(user.id),
    enableRowSelection: true,
    state: { rowSelection },
    onRowSelectionChange,
  })
  return (
    <>
      <table>
        <thead>
          {table.getHeaderGroups().map((group) => (
            <tr key={group.id}>
              {group.headers.map((header) => (
                <th key={header.id}>
                  {flexRender(
                    header.column.columnDef.header,
                    header.getContext()
                  )}
                </th>
              ))}
            </tr>
          ))}
        </thead>
        <tbody>
          {table.getRowModel().rows.map((row) => (
            <tr key={row.id}>
              {row.getVisibleCells().map((cell) => (
                <td key={cell.id}>
                  {flexRender(cell.column.columnDef.cell, cell.getContext())}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      <DataTableBulkActions table={table} />
    </>
  )
}

function renderUsers(role: number = ROLE.SUPER_ADMIN) {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'operator', role })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  clients.push(client)
  return render(
    <QueryClientProvider client={client}>
      <UsersProvider>
        <UsersListing users={users} />
      </UsersProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        scope: 'admin.user.batch.delete',
        methods: [{ method: '2fa', available: true }],
        oauth_providers: [],
        password_encryption_enabled: false,
      },
    },
  })
  vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/verify') {
      return {
        data: {
          success: true,
          data: {
            proof_token: 'single-use-proof',
            method: '2fa',
            scope: 'admin.user.batch.delete',
            expires_at: Math.floor(Date.now() / 1000) + 300,
          },
        },
      }
    }
    return { data: { success: true, data: { deleted: 2, skipped: 0 } } }
  })
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  useAuthStore.getState().auth.reset()
})

it('shows email and requires confirmation and a proof bound to the selected non-root users', async () => {
  const user = userEvent.setup()
  renderUsers()
  expect(screen.getByRole('columnheader', { name: 'Email' })).toBeVisible()
  expect(screen.getByRole('cell', { name: 'alice@example.com' })).toBeVisible()
  expect(screen.getByRole('cell', { name: 'bob@example.com' })).toBeVisible()
  await user.click(screen.getByRole('checkbox', { name: 'Select all' }))
  await user.click(
    screen.getByRole('button', { name: 'Delete selected users' })
  )
  const confirmation = await screen.findByRole('alertdialog')
  expect(confirmation).toHaveTextContent('Delete 2 user(s)?')
  expect(api.post).not.toHaveBeenCalled()
  await user.click(within(confirmation).getByRole('button', { name: 'Delete' }))
  const code = await screen.findByLabelText('Authenticator code or backup code')
  expect(api.post).not.toHaveBeenCalled()
  await user.type(code, '123456')
  await user.click(screen.getByRole('button', { name: 'Verify' }))
  await waitFor(() =>
    expect(api.post).toHaveBeenCalledWith(
      '/api/user/batch/delete',
      { ids: [7, 8] },
      expect.objectContaining({
        headers: { 'X-Security-Proof': 'single-use-proof' },
        singleUseAuthorization: true,
      })
    )
  )
  expect(api.post).toHaveBeenCalledWith(
    '/api/verify',
    expect.objectContaining({
      scope: 'admin.user.batch.delete',
      context: { user_ids: [7, 8] },
    }),
    expect.anything()
  )
  await waitFor(() =>
    expect(
      screen.getByRole('checkbox', { name: 'Select all' })
    ).not.toBeChecked()
  )
})

it('does not delete when confirmation or identity verification is cancelled', async () => {
  const user = userEvent.setup()
  renderUsers()
  await user.click(screen.getAllByRole('checkbox', { name: 'Select row' })[0])
  await user.click(
    screen.getByRole('button', { name: 'Delete selected users' })
  )
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(api.post).not.toHaveBeenCalled()
  await user.click(
    screen.getByRole('button', { name: 'Delete selected users' })
  )
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  await screen.findByLabelText('Authenticator code or backup code')
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  expect(api.post).not.toHaveBeenCalled()
  const confirmation = await screen.findByRole('alertdialog')
  await user.click(within(confirmation).getByRole('button', { name: 'Cancel' }))
  expect(
    screen.getAllByRole('checkbox', { name: 'Select row' })[0]
  ).toBeChecked()
})

it('disables deletion for root-only selections', async () => {
  renderUsers()
  await userEvent.click(
    screen.getAllByRole('checkbox', { name: 'Select row' })[2]
  )
  expect(
    screen.getByRole('button', { name: 'Delete selected users' })
  ).toBeDisabled()
})

it('hides batch deletion from administrators', async () => {
  renderUsers(ROLE.ADMIN)
  await userEvent.click(screen.getByRole('checkbox', { name: 'Select all' }))
  expect(
    screen.queryByRole('button', { name: 'Delete selected users' })
  ).not.toBeInTheDocument()
})

it('retains selected users and the confirmation after a rejected deletion', async () => {
  const user = userEvent.setup()
  const post = vi.mocked(api.post)
  const originalPost = post.getMockImplementation()
  if (!originalPost) {
    throw new Error('Missing API fixture')
  }
  post.mockImplementation(async (url, body, config) => {
    if (url === '/api/user/batch/delete') {
      return { data: { success: false, message: 'Invalid parameters' } }
    }
    return originalPost(url, body, config)
  })
  renderUsers()
  await user.click(screen.getAllByRole('checkbox', { name: 'Select row' })[0])
  await user.click(
    screen.getByRole('button', { name: 'Delete selected users' })
  )
  await user.click(screen.getByRole('button', { name: 'Delete' }))
  await user.type(
    await screen.findByLabelText('Authenticator code or backup code'),
    '123456'
  )
  await user.click(screen.getByRole('button', { name: 'Verify' }))
  const confirmation = await screen.findByRole('alertdialog')
  expect(confirmation).toHaveTextContent('Delete 1 user(s)?')
  await waitFor(() =>
    expect(
      within(confirmation).getByRole('button', { name: 'Delete' })
    ).toBeEnabled()
  )
  await user.click(within(confirmation).getByRole('button', { name: 'Cancel' }))
  expect(
    screen.getAllByRole('checkbox', { name: 'Select row' })[0]
  ).toBeChecked()
  expect(post).toHaveBeenCalledWith(
    '/api/user/batch/delete',
    { ids: [7] },
    expect.objectContaining({ singleUseAuthorization: true })
  )
})

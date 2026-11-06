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
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { RelatedAlertsLink } from '../related-alerts-link'

const original = useAuthStore.getState().auth

afterEach(() => {
  cleanup()
  useAuthStore.setState({ auth: original })
})

async function showLink(props: Parameters<typeof RelatedAlertsLink>[0]) {
  const route = createRootRoute({
    component: () => <RelatedAlertsLink {...props} />,
  })
  const router = createRouter({
    routeTree: route,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  render(<RouterProvider router={router} />)
}

it('opens channel alerts with the stable channel ID and platform ownership', async () => {
  useAuthStore.getState().auth.setUser({ id: 3, username: 'root', role: 100 })
  await showLink({ ownerID: 0, channelID: 192 })
  const href = (
    await screen.findByRole('link', { name: 'Alert center' })
  ).getAttribute('href')
  expect(href).toContain('channel_id=192')
  expect(href).toContain('owner_user_id=0')
})

it('keeps another account link hidden without the all-alerts permission', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'owner', role: 1 })
  await showLink({ ownerID: 2 })
  expect(screen.queryByRole('link')).not.toBeInTheDocument()
})

it('lets an owner open their own Key alerts without platform permission', async () => {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'owner', role: 1 })
  await showLink({ ownerID: 1, tokenID: 31 })
  const href = (
    await screen.findByRole('link', { name: 'Alert center' })
  ).getAttribute('href')
  expect(href).toContain('token_id=31')
  expect(href).toContain('owner_user_id=1')
})

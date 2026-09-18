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
import { cleanup, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { checkIsActive } from '@/components/layout/lib/url-utils'
import {
  parseSidebarModulesAdmin,
  serializeSidebarModulesAdmin,
} from '@/features/system-settings/maintenance/config'
import { useAuthStore } from '@/stores/auth-store'

import { useSidebarConfig } from '../use-sidebar-config'
import { useSidebarData } from '../use-sidebar-data'

beforeEach(() => {
  vi.stubGlobal('localStorage', {
    getItem: () => null,
    setItem: () => undefined,
    removeItem: () => undefined,
  })
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  useAuthStore.getState().auth.reset()
})

function sidebarFor(admin?: object, user?: object, canConfigure = true) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(['status'], {
    SidebarModulesAdmin: admin ? JSON.stringify(admin) : '',
  })
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'alice',
    role: 1,
    permissions: { sidebar_settings: canConfigure },
    sidebar_modules: user ? JSON.stringify(user) : '',
  })
  function Wrapper(props: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>
        {props.children}
      </QueryClientProvider>
    )
  }
  const result = renderHook(
    () => useSidebarConfig(useSidebarData().navGroups),
    { wrapper: Wrapper }
  )
  return result
}

describe('security sidebar visibility', () => {
  it('old configurations show Security & Access immediately after Profile and keep API Keys', () => {
    const { result } = sidebarFor(
      { personal: { enabled: true, personal: true, topup: true } },
      { personal: { enabled: true, personal: true } }
    )
    expect(
      result.current
        .find((group) => group.id === 'personal')
        ?.items.map((item) => item.title)
    ).toEqual(['Wallet', 'Profile', 'Security & Access'])
    expect(
      result.current
        .flatMap((group) => group.items)
        .some((item) => item.title === 'API Keys')
    ).toBe(true)
  })
  it.each([
    [{ personal: { enabled: true, security: false } }, undefined],
    [{ personal: { enabled: false } }, { personal: { security: true } }],
    [undefined, { personal: { enabled: true, security: false } }],
    [undefined, { personal: { enabled: false } }],
  ])(
    'admin or user disablement hides Security & Access (%j, %j)',
    (admin, user) => {
      const { result } = sidebarFor(admin, user)
      expect(
        result.current
          .flatMap((group) => group.items)
          .some((item) => item.title === 'Security & Access')
      ).toBe(false)
    }
  )
  it('users without sidebar configuration permission retain the admin view', () => {
    const { result } = sidebarFor(
      undefined,
      { personal: { security: false } },
      false
    )
    expect(
      result.current
        .flatMap((group) => group.items)
        .some((item) => item.title === 'Security & Access')
    ).toBe(true)
  })
})

describe('audit log sidebar entry', () => {
  it('admin settings default Audit Logs to visible and preserve its independent toggle when saved', () => {
    const config = parseSidebarModulesAdmin(
      '{"console":{"enabled":true,"log":true}}'
    )
    expect(config.console.audit).toBe(true)
    config.console.audit = false
    const saved = parseSidebarModulesAdmin(serializeSidebarModulesAdmin(config))
    const { result } = sidebarFor(saved)
    const titles = result.current
      .flatMap((group) => group.items)
      .map((item) => item.title)
    expect(titles).not.toContain('Audit Logs')
    expect(titles).toContain('Usage Logs')
  })

  it('legacy configurations show a separate Audit Logs link immediately after Usage Logs', () => {
    const { result } = sidebarFor(
      { console: { enabled: true, log: true } },
      { console: { enabled: true, log: true } }
    )
    const items =
      result.current.find((group) => group.id === 'general')?.items ?? []
    const usageIndex = items.findIndex((item) => item.title === 'Usage Logs')
    expect(items[usageIndex + 1]).toMatchObject({
      title: 'Audit Logs',
      url: '/usage-logs/audit',
    })
    const selected = items.filter((item) =>
      checkIsActive('/usage-logs/audit', item)
    )
    expect(selected.map((item) => item.title)).toEqual(['Audit Logs'])
  })

  it.each([
    [{ console: { enabled: true, audit: false } }, undefined],
    [{ console: { enabled: false } }, { console: { audit: true } }],
    [undefined, { console: { enabled: true, audit: false } }],
    [undefined, { console: { enabled: false } }],
  ])(
    'admin and personal visibility rules can hide Audit Logs (%j, %j)',
    (admin, user) => {
      const { result } = sidebarFor(admin, user)
      expect(
        result.current
          .flatMap((group) => group.items)
          .some((item) => item.title === 'Audit Logs')
      ).toBe(false)
    }
  )

  it('hiding Usage Logs does not hide the independently configured Audit Logs entry', () => {
    const { result } = sidebarFor({ console: { enabled: true, log: false } })
    const titles = result.current
      .flatMap((group) => group.items)
      .map((item) => item.title)
    expect(titles).not.toContain('Usage Logs')
    expect(titles).toContain('Audit Logs')
  })
})

describe('merged sidebar navigation', () => {
  it('keeps the existing billing and administration entries alongside new security entries', () => {
    const { result } = sidebarFor()
    const routesByGroup = Object.fromEntries(
      result.current.map((group) => [
        group.id,
        group.items.flatMap((item) => ('url' in item ? [item.url] : [])),
      ])
    )

    expect(routesByGroup.general).toEqual([
      '/dashboard/overview',
      '/dashboard/models',
      '/keys',
      '/usage-logs/common',
      '/usage-logs/audit',
      '/error-logs',
      '/usage-logs/task',
    ])
    expect(routesByGroup.personal).toEqual(['/wallet', '/profile', '/security'])
    expect(routesByGroup.billing).toEqual([
      '/model-usage-analysis',
      '/quota-statistics',
      '/bill',
      '/client-user-quota',
      '/project-budget',
      '/settlement-config',
      '/pricing-center',
    ])
    expect(routesByGroup.admin).toEqual([
      '/channels',
      '/models/metadata',
      '/models/deployments',
      '/users',
      '/redemption-codes',
      '/subscriptions',
      '/model-channel-monitor',
      '/internal-channel-monitor',
      '/model-route-config',
      '/sync-environments',
      '/system-info',
      '/task-plugins',
      '/system-settings/site',
    ])
  })

  it('restores legacy module switches without losing audit and security switches', () => {
    const config = parseSidebarModulesAdmin('{}')

    expect(config.console).toMatchObject({
      audit: true,
      errorlog: true,
      quotaStatistics: true,
      modelUsageAnalysis: true,
      billQuery: false,
      billPricing: false,
      billDiscount: true,
    })
    expect(config.personal).toMatchObject({
      security: true,
      accountManagement: true,
      preferences: true,
    })
    expect(config.admin).toMatchObject({
      deployment: true,
      modelRouteConfig: true,
      modelChannelMonitor: true,
      internalChannelMonitor: true,
      cuquota: true,
      project: true,
      settlementConfig: true,
      pricingCenter: true,
    })
  })

  it('keeps page whitelist keys on restored entries', () => {
    const { result } = sidebarFor()
    const items = result.current.flatMap((group) => group.items)
    const byUrl = (url: string) =>
      items.find((item) => 'url' in item && item.url === url)

    expect(byUrl('/bill')?.pageKeys).toEqual(['bill', 'bill_self'])
    expect(byUrl('/models/deployments')?.pageKeys).toEqual(['deployment'])
    expect(byUrl('/model-route-config')?.pageKeys).toEqual([
      'model_route_config',
    ])
  })

  it('keeps the model deployment switch independent from the model catalog', () => {
    const { result } = sidebarFor({
      admin: { enabled: true, models: true, deployment: false },
    })
    const routes = result.current
      .find((group) => group.id === 'admin')
      ?.items.flatMap((item) => ('url' in item ? [item.url] : []))

    expect(routes).toContain('/models/metadata')
    expect(routes).not.toContain('/models/deployments')
  })
})

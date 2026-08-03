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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { NavGroup } from '@/components/layout/types'

import { allowedUrlsForPages, whitelistRedirectTarget } from '../org-whitelist'

const navGroups = [
  {
    id: 'console',
    title: 'Console',
    items: [
      { title: 'Overview', url: '/dashboard/overview' },
      { title: 'Usage Logs', url: '/usage-logs/common', pageKeys: ['log'] },
      {
        title: 'Quota Statistics',
        url: '/quota-statistics',
        pageKeys: ['quota_statistics'],
      },
      { title: 'Bill', url: '/bill', pageKeys: ['bill', 'bill_self'] },
    ],
  },
  {
    id: 'admin',
    title: 'Admin',
    items: [
      { title: 'UID Budgets', url: '/client-user-quota', pageKeys: ['client_user_quota'] },
    ],
  },
] as unknown as NavGroup[]

describe('allowedUrlsForPages', () => {
  test('returns granted routes in sidebar order', () => {
    assert.deepEqual(
      allowedUrlsForPages(navGroups, ['client_user_quota', 'log']),
      ['/usage-logs/common', '/client-user-quota']
    )
  })

  test('matches an item when any of its page keys is granted', () => {
    // The bill entry covers a regular user's own bill and the org-wide one.
    assert.deepEqual(allowedUrlsForPages(navGroups, ['bill_self']), ['/bill'])
    assert.deepEqual(allowedUrlsForPages(navGroups, ['bill']), ['/bill'])
  })

  test('ignores items with no page key, which are not org-gated', () => {
    assert.deepEqual(allowedUrlsForPages(navGroups, []), [])
  })
})

describe('whitelistRedirectTarget', () => {
  const allowed = ['/usage-logs/common', '/quota-statistics']

  test('sends an mt account off the dashboard to its first allowed page', () => {
    assert.equal(
      whitelistRedirectTarget('/dashboard/overview', allowed),
      '/usage-logs/common'
    )
  })

  test('stays put on an allowed page', () => {
    assert.equal(whitelistRedirectTarget('/quota-statistics', allowed), null)
  })

  test('treats sub-routes of an allowed page as allowed', () => {
    assert.equal(
      whitelistRedirectTarget('/usage-logs/common/detail', allowed),
      null
    )
  })

  test('does nothing when nothing is granted, rather than looping', () => {
    assert.equal(whitelistRedirectTarget('/dashboard/overview', []), null)
  })
})

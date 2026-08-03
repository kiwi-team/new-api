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
import type { NavGroup } from '@/components/layout/types'

/**
 * Routes an org whitelist grants, in sidebar order.
 *
 * Derived from the sidebar definition rather than a second hand-written table:
 * the page key to URL mapping already lives on the nav items, and keeping a
 * parallel copy is how it drifts.
 */
export function allowedUrlsForPages(
  navGroups: NavGroup[],
  pages: string[]
): string[] {
  const granted = new Set(pages)
  const urls: string[] = []

  for (const group of navGroups) {
    for (const item of group.items) {
      const keys = item.pageKeys
      if (!keys || keys.length === 0) continue
      if (!keys.some((key) => granted.has(key))) continue
      const url = 'url' in item ? item.url : undefined
      if (typeof url === 'string' && !urls.includes(url)) urls.push(url)
    }
  }

  return urls
}

/**
 * Where a whitelist-mode account should be sent, or `null` to stay put.
 *
 * Accounts in "logout only" mode (mt, see org.md) may only reach the pages
 * their org grants. Landing anywhere else — the default `/dashboard/overview`
 * after sign-in, or a pasted URL — bounces them to their first allowed page.
 * A prefix match is used because granted pages have sub-routes.
 */
export function whitelistRedirectTarget(
  currentPath: string,
  allowedUrls: string[]
): string | null {
  if (allowedUrls.length === 0) return null
  if (allowedUrls.some((url) => currentPath.startsWith(url))) return null
  return allowedUrls[0]
}

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
import { useLocation, useNavigate } from '@tanstack/react-router'
import { useEffect } from 'react'

import { allowedUrlsForPages, whitelistRedirectTarget } from './lib/org-whitelist'
import { useSidebarData } from './use-sidebar-data'
import { useUserMenu } from './use-user-menu'

/**
 * Keep "logout only" org accounts inside their granted pages.
 *
 * These accounts (mt, see org.md) have no dashboard, so the post-sign-in
 * landing route and any pasted URL outside the whitelist both bounce to their
 * first allowed page. The redirect waits for the menu to load — `pages` is
 * `null` until then — so a slow request never bounces someone off a page they
 * are in fact allowed to see.
 *
 * This is a navigation convenience, not an authorization boundary; the backend
 * `PageAuth` middleware is what actually denies access.
 */
export function useOrgWhitelistRedirect(): void {
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const { pages, topbarMode, isPrivileged } = useUserMenu()
  const { navGroups } = useSidebarData()

  useEffect(() => {
    if (isPrivileged || topbarMode !== 'logout_only' || pages === null) return

    const target = whitelistRedirectTarget(
      pathname,
      allowedUrlsForPages(navGroups, pages)
    )
    if (target) void navigate({ to: target, replace: true })
  }, [isPrivileged, topbarMode, pages, pathname, navGroups, navigate])
}

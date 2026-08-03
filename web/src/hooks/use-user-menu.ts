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
import { useQuery } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

/**
 * Page keys returned by `GET /api/user/menu`.
 *
 * The values are the backend page constants in `service/org_view.go`
 * (`log`, `quota_statistics`, `client_user_quota`, …). They are a string
 * contract, not route paths — nav items opt in via `NavItem.pageKey`.
 */
export type UserMenu = {
  /** `logout_only` strips the header down to a sign-out action */
  topbar_mode: 'normal' | 'logout_only'
  pages: string[]
}

async function getUserMenu(): Promise<UserMenu | null> {
  const res = await api.get('/api/user/menu')
  if (!res.data?.success) return null
  return res.data.data as UserMenu
}

/**
 * Organization menu whitelist for the signed-in user.
 *
 * Org-tagged accounts only see the pages their organization grants. System
 * admins and root bypass the whitelist entirely, so the request is skipped
 * for them. While the menu is still loading, `pages` is `null` and callers
 * must not filter anything — hiding first and revealing later flickers.
 */
export function useUserMenu(): {
  pages: string[] | null
  topbarMode: UserMenu['topbar_mode']
  isPrivileged: boolean
} {
  const role = useAuthStore((s) => s.auth.user?.role)
  const isPrivileged = (role ?? ROLE.GUEST) >= ROLE.ADMIN
  const isAuthenticated = role !== undefined

  const { data } = useQuery({
    queryKey: ['user-menu'],
    queryFn: getUserMenu,
    enabled: isAuthenticated && !isPrivileged,
    retry: false,
    staleTime: 5 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
  })

  return {
    pages: data?.pages ?? null,
    topbarMode: data?.topbar_mode ?? 'normal',
    isPrivileged,
  }
}

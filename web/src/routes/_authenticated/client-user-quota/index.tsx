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
import { createFileRoute, redirect } from '@tanstack/react-router'
import z from 'zod'

import { ClientUserQuotaPage } from '@/features/client-user-quota'
import { useAuthStore } from '@/stores/auth-store'

const clientUserQuotaSearchSchema = z.object({
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(undefined),
  filter: z.string().optional().catch(''),
})

export const Route = createFileRoute('/_authenticated/client-user-quota/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()

    // Org-tagged non-admin accounts can be granted this page through
    // `/api/user/menu`; the API enforces scope via `PageAuth`.
    if (!auth.user) {
      throw redirect({ to: '/403' })
    }
  },
  validateSearch: clientUserQuotaSearchSchema,
  component: ClientUserQuotaPage,
})

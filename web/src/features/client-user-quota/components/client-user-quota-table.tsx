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
import { getRouteApi } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getBatchProjectBudget, getClientUserQuotas } from '../api'
import { useClientUserQuotaColumns } from './client-user-quota-columns'
import { useClientUserQuota } from './client-user-quota-provider'

const route = getRouteApi('/_authenticated/client-user-quota/')

export function ClientUserQuotaTable() {
  const { t } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const { refreshTrigger, setOpen, setCurrentRow } = useClientUserQuota()
  const role = useAuthStore((state) => state.auth.user?.role) ?? ROLE.GUEST

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
  })

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'client-user-quota',
      pagination.pageIndex + 1,
      pagination.pageSize,
      globalFilter,
      refreshTrigger,
    ],
    queryFn: async () => {
      const result = await getClientUserQuotas({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        keyword: globalFilter,
      })
      if (!result.success) {
        toast.error(result.message || t('Failed to load'))
        return { items: [], total: 0 }
      }
      return {
        items: result.data?.items ?? [],
        total: result.data?.total ?? 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const quotas = data?.items ?? []

  // One rollup request per page of UIDs instead of one per row.
  const projectBudgetsQuery = useQuery({
    queryKey: [
      'client-user-quota',
      'project-budgets',
      quotas.map((quota) => quota.client_user_id).join(','),
    ],
    queryFn: () =>
      getBatchProjectBudget(quotas.map((quota) => quota.client_user_id)),
    enabled: quotas.length > 0,
  })

  const columns = useClientUserQuotaColumns({
    showClientName: role >= ROLE.ADMIN,
    projectBudgets: projectBudgetsQuery.data ?? {},
    onOpenProjectBudget: (clientUserId) => {
      setCurrentRow(
        quotas.find((quota) => quota.client_user_id === clientUserId) ?? null
      )
      setOpen('project-budget')
    },
  })

  const { table } = useDataTable({
    data: quotas,
    columns,
    globalFilter,
    columnFilters,
    pagination,
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: data?.total ?? 0,
    ensurePageInRange,
  })

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No UID budgets found')}
      emptyDescription={t(
        'No UID budgets have been created yet. Add one to start allocating budget.'
      )}
      skeletonKeyPrefix='client-user-quota-skeleton'
      applyHeaderSize
      toolbarProps={{
        searchPlaceholder: t('Search by client UID'),
      }}
    />
  )
}

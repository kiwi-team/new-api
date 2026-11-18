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
import type { ColumnDef } from '@tanstack/react-table'
import { useEffect, useRef, useState, type ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePagination, useDataTable } from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import { PageFooterPortal } from '@/components/layout/components/page-footer'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { useMediaQuery } from '@/hooks/use-media-query'

import { alertRequest } from '../api'
import { alertLabel } from '../lib/labels'
import type { AlertRule, AlertScope } from '../types'
import { RulesTable } from './rules-table'

type Filters = {
  q: string
  status: string
  scope_type: string
  metric: string
  severity: string
  management_mode: string
}
const EMPTY_FILTERS: Filters = {
  q: '',
  status: '',
  scope_type: '',
  metric: '',
  severity: '',
  management_mode: '',
}
const EMPTY_RULES: AlertRule[] = []
const EMPTY_COLUMNS: ColumnDef<AlertRule, unknown>[] = []

type Props = Omit<ComponentProps<typeof RulesTable>, 'rules'> & {
  baseParams: string
  scopes: AlertScope[]
  metrics: string[]
}
type RulePage = {
  items: AlertRule[]
  total: number
  page: number
  page_size: number
}

export function RulesList(props: Props) {
  const { t } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const [filters, setFilters] = useState(EMPTY_FILTERS)
  const [searchInput, setSearchInput] = useState('')
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 20 })
  const tableStart = useRef<HTMLDivElement>(null)
  const scrollOnPageLoad = useRef(false)
  const params = new URLSearchParams(props.baseParams)
  params.delete('before_id')
  params.set('page', String(pagination.pageIndex + 1))
  params.set('limit', String(pagination.pageSize))
  for (const [key, value] of Object.entries(filters)) {
    if (value) params.set(key, value)
    else params.delete(key)
  }
  const suffix = params.toString()
  const query = useQuery({
    queryKey: ['alerts', 'rules', suffix],
    queryFn: () => alertRequest<RulePage>(`rules?${suffix}`),
    refetchInterval: 15000,
  })
  useEffect(() => {
    if (!query.isSuccess || !scrollOnPageLoad.current) return
    tableStart.current?.scrollIntoView({ block: 'start', behavior: 'instant' })
    scrollOnPageLoad.current = false
  }, [query.isSuccess, suffix])
  const currentPagination = {
    pageIndex: query.data ? query.data.page - 1 : pagination.pageIndex,
    pageSize: pagination.pageSize,
  }
  const { table } = useDataTable({
    data: query.data?.items ?? EMPTY_RULES,
    columns: EMPTY_COLUMNS,
    totalCount: query.data?.total ?? 0,
    pagination: currentPagination,
    onPaginationChange: (updater) => {
      if (query.isFetching || query.isError) return
      const next =
        typeof updater === 'function' ? updater(currentPagination) : updater
      if (
        next.pageIndex === currentPagination.pageIndex &&
        next.pageSize === currentPagination.pageSize
      ) {
        return
      }
      scrollOnPageLoad.current = true
      setPagination({
        ...next,
        pageIndex: next.pageSize === pagination.pageSize ? next.pageIndex : 0,
      })
    },
    manualPagination: true,
    manualFiltering: true,
    enableRowSelection: false,
    enableSorting: false,
  })
  const updateFilters = (patch: Partial<Filters>) => {
    setFilters((previous) => ({ ...previous, ...patch }))
    setPagination((previous) => ({ ...previous, pageIndex: 0 }))
  }
  const selects: {
    field: keyof Filters
    label: string
    allLabel: string
    options: string[]
  }[] = [
    {
      field: 'status',
      label: t('Status'),
      allLabel: t('All statuses'),
      options: ['enabled', 'paused'],
    },
    {
      field: 'scope_type',
      label: t('Monitored object'),
      allLabel: t('All monitored objects'),
      options: props.scopes,
    },
    {
      field: 'metric',
      label: t('Metric'),
      allLabel: t('All metrics'),
      options: props.metrics,
    },
    {
      field: 'severity',
      label: t('Severity'),
      allLabel: t('All severities'),
      options: ['P0', 'P1', 'P2', 'P3'],
    },
    {
      field: 'management_mode',
      label: t('Management mode'),
      allLabel: t('All management modes'),
      options: ['user_managed', 'platform_enforced'],
    },
  ]
  return (
    <section
      className='space-y-3'
      aria-label={t('Rules')}
      aria-busy={query.isFetching}
    >
      <form
        className='flex flex-wrap gap-2'
        onSubmit={(event) => {
          event.preventDefault()
          updateFilters({ q: searchInput.trim() })
        }}
      >
        <Input
          className='min-w-48 flex-1'
          aria-label={t('Search by name or ID')}
          placeholder={t('Search by name or ID')}
          value={searchInput}
          maxLength={160}
          onChange={(event) => setSearchInput(event.target.value)}
        />
        <Button variant='outline' type='submit'>
          {t('Search')}
        </Button>
        <Button
          variant='outline'
          type='button'
          onClick={() => {
            setSearchInput('')
            updateFilters(EMPTY_FILTERS)
          }}
        >
          {t('Clear filters')}
        </Button>
      </form>
      <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-5'>
        {selects.map((select) => (
          <label key={select.field} className='min-w-0 space-y-1 text-sm'>
            <span>{select.label}</span>
            <NativeSelect
              className='w-full'
              value={filters[select.field]}
              onChange={(event) =>
                updateFilters({ [select.field]: event.target.value })
              }
            >
              <NativeSelectOption value=''>
                {select.allLabel}
              </NativeSelectOption>
              {select.options.map((value) => (
                <NativeSelectOption key={value} value={value}>
                  {select.field === 'status'
                    ? t(value === 'enabled' ? 'Enabled' : 'Paused')
                    : alertLabel(value, t)}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </label>
        ))}
      </div>
      {query.isError ? (
        <ErrorState onRetry={() => void query.refetch()} />
      ) : (
        <>
          {query.isPending ? (
            <LoadingState />
          ) : (
            <div ref={tableStart}>
              <RulesTable
                rules={query.data.items}
                canManage={props.canManage}
                onView={props.onView}
                onEdit={props.onEdit}
                onPause={props.onPause}
                onDelete={props.onDelete}
              />
            </div>
          )}
          <PageFooterPortal>
            <fieldset
              disabled={query.isFetching}
              aria-label={t('Page')}
              className='min-w-0'
            >
              <DataTablePagination table={table} compact={isMobile} />
            </fieldset>
          </PageFooterPortal>
        </>
      )}
    </section>
  )
}

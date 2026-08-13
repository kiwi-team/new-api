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
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Download, Loader2, Search } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table'
import { DateTimePicker } from '@/components/datetime-picker'
import { MultiSelect } from '@/components/multi-select'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Toggle } from '@/components/ui/toggle'
import { useDebounce } from '@/hooks/use-debounce'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { searchUserOptions } from '@/features/settlement-config/api'

import {
  exportQuotaStatisticsCsv,
  getProjectNames,
  getQuotaStatistics,
  getTokenOptions,
} from './api'
import { useQuotaStatisticsColumns } from './components/quota-statistics-columns'
import {
  SCENARIO_VALUES,
  currentMonthRange,
  getScenarioLabel,
  statisticsRowKey,
} from './lib'
import type { QuotaStatisticsQuery } from './types'

const ALL_USERS = '0'
const ALL_PROJECTS = 'all'

export function QuotaStatisticsPage() {
  const { t } = useTranslation()
  const role = useAuthStore((state) => state.auth.user?.role) ?? ROLE.GUEST
  const isRoot = role >= ROLE.SUPER_ADMIN

  const defaultRange = useMemo(currentMonthRange, [])
  const [startTime, setStartTime] = useState<Date | undefined>(defaultRange.start)
  const [endTime, setEndTime] = useState<Date | undefined>(defaultRange.end)
  const [expandModels, setExpandModels] = useState(false)
  const [expandDates, setExpandDates] = useState(false)
  const [expandTokens, setExpandTokens] = useState(false)
  const [modelName, setModelName] = useState('')
  const [clientUserId, setClientUserId] = useState('')
  const [projectName, setProjectName] = useState(ALL_PROJECTS)
  const [scenarios, setScenarios] = useState<string[]>([])
  const [userId, setUserId] = useState(ALL_USERS)
  const [tokenIds, setTokenIds] = useState<string[]>([])
  const [userKeyword, setUserKeyword] = useState('')
  const debouncedUserKeyword = useDebounce(userKeyword, 300)
  // Typed filters reach the query only once typing settles, so that a request
  // is not fired per keystroke. The Query button commits them immediately.
  const [committedText, setCommittedText] = useState({ model: '', uid: '' })
  useEffect(() => {
    const handler = setTimeout(
      () => setCommittedText({ model: modelName, uid: clientUserId }),
      400
    )
    return () => clearTimeout(handler)
  }, [modelName, clientUserId])

  const tokensQuery = useQuery({
    queryKey: ['quota-statistics', 'tokens'],
    queryFn: getTokenOptions,
  })
  const projectsQuery = useQuery({
    queryKey: ['quota-statistics', 'projects'],
    queryFn: getProjectNames,
  })
  const usersQuery = useQuery({
    queryKey: ['quota-statistics', 'users', debouncedUserKeyword],
    queryFn: () => searchUserOptions(debouncedUserKeyword),
    enabled: isRoot,
  })

  // Every filter change re-derives the query, so the table refreshes on its
  // own. `null` means the range is incomplete and nothing can be asked.
  const query = useMemo<QuotaStatisticsQuery | null>(() => {
    if (!startTime || !endTime) return null
    return {
      start_timestamp: Math.floor(startTime.getTime() / 1000),
      end_timestamp: Math.floor(endTime.getTime() / 1000),
      model_name: committedText.model,
      client_user_id: committedText.uid,
      client_scenairos: scenarios.join(','),
      expand_models: expandModels,
      expand_dates: expandDates,
      expand_tokens: expandTokens,
      ...(isRoot && userId !== ALL_USERS
        ? { user_id: Number.parseInt(userId, 10) }
        : {}),
      ...(projectName === ALL_PROJECTS ? {} : { project_name: projectName }),
      ...(tokenIds.length > 0 ? { token_ids: tokenIds.join(',') } : {}),
    }
  }, [
    startTime,
    endTime,
    committedText,
    scenarios,
    expandModels,
    expandDates,
    expandTokens,
    isRoot,
    userId,
    projectName,
    tokenIds,
  ])

  const statisticsQuery = useQuery({
    queryKey: ['quota-statistics', 'rows', query],
    // The params travel with the result so that the rendered columns always
    // describe the rows on screen, not a newer filter that is still loading.
    queryFn: async () => {
      const params = query as QuotaStatisticsQuery
      return { params, rows: await getQuotaStatistics(params) }
    },
    enabled: query !== null,
    placeholderData: keepPreviousData,
  })

  const rows = statisticsQuery.data?.rows ?? []
  const totalUSD = rows.reduce(
    (total, row) => total + (Number(row.total_quota) || 0),
    0
  )

  const columns = useQuotaStatisticsColumns({
    models: statisticsQuery.data?.params.expand_models ?? false,
    dates: statisticsQuery.data?.params.expand_dates ?? false,
    tokens: statisticsQuery.data?.params.expand_tokens ?? false,
  })

  const handleExport = async () => {
    if (!query) {
      toast.warning(t('Please select a time range'))
      return
    }
    try {
      // Export follows the form as displayed, including text still inside its
      // debounce window.
      const blob = await exportQuotaStatisticsCsv({
        ...query,
        model_name: modelName,
        client_user_id: clientUserId,
      })
      const url = window.URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = 'quota_statistics.csv'
      link.click()
      window.URL.revokeObjectURL(url)
    } catch {
      toast.error(t('Export failed'))
    }
  }

  const projectItems = [
    { value: ALL_PROJECTS, label: t('All projects') },
    ...(projectsQuery.data ?? []).map((name) => ({ value: name, label: name })),
  ]
  const userItems = [
    { value: ALL_USERS, label: t('All users') },
    ...(usersQuery.data ?? []).map((option) => ({
      value: String(option.value),
      label: option.label,
    })),
  ]
  // Keys are searchable by name or by the key itself, matching the old page.
  const tokenOptions = (tokensQuery.data ?? []).map((token) => ({
    value: String(token.id),
    label: `${token.name} (ID: ${token.id})${token.key ? ` ${token.key}` : ''}`,
  }))

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Quota Statistics')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-4'>
          <div className='flex flex-wrap items-end gap-3'>
            <div className='grid gap-1.5'>
              <Label>{t('Start Time')}</Label>
              <DateTimePicker value={startTime} onChange={setStartTime} />
            </div>
            <div className='grid gap-1.5'>
              <Label>{t('End Time')}</Label>
              <DateTimePicker value={endTime} onChange={setEndTime} />
            </div>
            <Badge variant='outline' className='mb-2'>
              {t('Total consumption')}: ${totalUSD.toFixed(2)}
            </Badge>
            <div className='mb-0.5 flex flex-wrap gap-2'>
              <Toggle pressed={expandModels} onPressedChange={setExpandModels}>
                {t('Expand by model')}
              </Toggle>
              <Toggle pressed={expandDates} onPressedChange={setExpandDates}>
                {t('Expand by date')}
              </Toggle>
              <Toggle pressed={expandTokens} onPressedChange={setExpandTokens}>
                {t('Expand by key')}
              </Toggle>
            </div>
          </div>

          <div className='flex flex-wrap items-end gap-3'>
            <div className='grid gap-1.5'>
              <Label htmlFor='qs-model'>{t('Model name')}</Label>
              <Input
                id='qs-model'
                className='w-40'
                value={modelName}
                onChange={(event) => setModelName(event.target.value)}
              />
            </div>
            <div className='grid gap-1.5'>
              <Label htmlFor='qs-uid'>{t('Client UID')}</Label>
              <Input
                id='qs-uid'
                className='w-40'
                value={clientUserId}
                onChange={(event) => setClientUserId(event.target.value)}
              />
            </div>
            <div className='grid gap-1.5'>
              <Label>{t('Project name')}</Label>
              <Select
                items={projectItems}
                value={projectName}
                onValueChange={(value) => setProjectName(value ?? ALL_PROJECTS)}
              >
                <SelectTrigger className='w-44'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {projectItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className='grid gap-1.5'>
              <Label>{t('Scenario')}</Label>
              <MultiSelect
                className='w-56'
                options={SCENARIO_VALUES.map((value) => ({
                  value,
                  label: getScenarioLabel(value, t),
                }))}
                selected={scenarios}
                onChange={setScenarios}
                placeholder={t('Scenario')}
              />
            </div>
            {isRoot && (
              <div className='grid gap-1.5'>
                <Label>{t('User')}</Label>
                <Select
                  items={userItems}
                  value={userId}
                  onValueChange={(value) => setUserId(value ?? ALL_USERS)}
                >
                  <SelectTrigger className='w-44'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <div className='p-1'>
                      <Input
                        placeholder={t('Search users')}
                        value={userKeyword}
                        onChange={(event) => setUserKeyword(event.target.value)}
                      />
                    </div>
                    {userItems.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
            <div className='grid gap-1.5'>
              <Label>{t('Key')}</Label>
              <MultiSelect
                className='w-56'
                options={tokenOptions}
                selected={tokenIds}
                onChange={setTokenIds}
                placeholder={t('Select keys')}
                maxVisibleChips={1}
              />
            </div>
            <Button
              className='mb-0.5'
              size='sm'
              onClick={() => {
                if (!startTime || !endTime) {
                  toast.warning(t('Please select a time range'))
                  return
                }
                // Filters already refresh on their own, so the button only has
                // to flush text still sitting in its debounce window. If that
                // text is already committed the query key is unchanged and
                // nothing would reload, so refetch forces the request instead.
                if (
                  modelName !== committedText.model ||
                  clientUserId !== committedText.uid
                ) {
                  setCommittedText({ model: modelName, uid: clientUserId })
                  return
                }
                void statisticsQuery.refetch()
              }}
              disabled={statisticsQuery.isFetching}
            >
              {statisticsQuery.isFetching ? (
                <Loader2 className='h-4 w-4 animate-spin' />
              ) : (
                <Search className='h-4 w-4' />
              )}
              {t('Query')}
            </Button>
            <Button
              className='mb-0.5'
              variant='outline'
              size='sm'
              onClick={() => void handleExport()}
            >
              <Download className='h-4 w-4' />
              {t('Export CSV')}
            </Button>
          </div>

          <div className='min-h-0 flex-1 overflow-auto'>
            <StaticDataTable
              tableClassName='min-w-max'
              data={rows}
              getRowKey={statisticsRowKey}
              emptyContent={t('No consumption data for this range')}
              emptyClassName='text-muted-foreground py-8'
              columns={columns}
            />
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

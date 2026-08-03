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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { Plug, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatTimestampToDate } from '@/lib/format'

import {
  deleteSyncEnvironment,
  getSyncEnvironments,
  testSyncEnvironment,
} from './api'
import { SyncEnvironmentMutateDialog } from './components/sync-environment-mutate-dialog'
import { SyncLogsTab } from './components/sync-logs-tab'
import { SYNC_ENVIRONMENT_STATUS, type SyncEnvironment } from './types'

const route = getRouteApi('/_authenticated/sync-environments/')

export function SyncEnvironmentsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const activeTab = search.tab ?? 'environments'
  const [mutateState, setMutateState] = useState<{
    currentRow?: SyncEnvironment
  } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<SyncEnvironment | null>(null)
  const [testingId, setTestingId] = useState<number | null>(null)

  const environmentsQuery = useQuery({
    queryKey: ['sync-environments', 'list'],
    queryFn: getSyncEnvironments,
  })

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['sync-environments'] })

  const handleTest = async (environment: SyncEnvironment) => {
    setTestingId(environment.id)
    try {
      const res = await testSyncEnvironment(environment.id)
      if (!res.success) {
        toast.error(res.message || t('Connection test failed'))
        return
      }
      toast.success(t('Connection test succeeded'))
    } finally {
      setTestingId(null)
    }
  }

  const handleDelete = async (environment: SyncEnvironment) => {
    setDeleteTarget(null)
    const res = await deleteSyncEnvironment(environment.id)
    if (!res.success) {
      toast.error(res.message || t('Operation failed'))
      return
    }
    toast.success(t('Deleted successfully'))
    await refresh()
  }

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('Environments')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <div className='flex items-center gap-2'>
            <Button
              variant='outline'
              size='sm'
              onClick={() => void refresh()}
              disabled={environmentsQuery.isFetching}
            >
              <RefreshCw className='h-4 w-4' />
              {t('Refresh')}
            </Button>
            <Button size='sm' onClick={() => setMutateState({})}>
              <Plus className='h-4 w-4' />
              {t('Add environment')}
            </Button>
          </div>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex h-full min-h-0 flex-col gap-4'>
            <Tabs
              value={activeTab}
              onValueChange={(tab) =>
                void navigate({
                  search: { tab: tab as 'environments' | 'logs' },
                })
              }
            >
              <TabsList>
                <TabsTrigger value='environments'>
                  {t('Environments')}
                </TabsTrigger>
                <TabsTrigger value='logs'>{t('Sync history')}</TabsTrigger>
              </TabsList>
            </Tabs>

            <div className='min-h-0 flex-1 overflow-auto'>
              {activeTab === 'logs' ? (
                <SyncLogsTab />
              ) : (
                <StaticDataTable
                  tableClassName='min-w-max'
                  data={environmentsQuery.data ?? []}
                  getRowKey={(environment) => environment.id}
                  emptyContent={t('No environments configured')}
                  emptyClassName='text-muted-foreground py-8'
                  columns={[
                    {
                      id: 'id',
                      header: t('ID'),
                      cell: (environment) => environment.id,
                    },
                    {
                      id: 'name',
                      header: t('Environment name'),
                      cellClassName: 'font-medium',
                      cell: (environment) => environment.name,
                    },
                    {
                      id: 'url',
                      header: t('API URL'),
                      cell: (environment) => environment.api_url,
                    },
                    {
                      id: 'user',
                      header: t('New-API user ID'),
                      cell: (environment) => environment.new_api_user || '-',
                    },
                    {
                      id: 'status',
                      header: t('Status'),
                      cell: (environment) => (
                        <StatusBadge
                          label={
                            environment.status ===
                            SYNC_ENVIRONMENT_STATUS.ENABLED
                              ? t('Enabled')
                              : t('Disabled')
                          }
                          variant={
                            environment.status ===
                            SYNC_ENVIRONMENT_STATUS.ENABLED
                              ? 'success'
                              : 'neutral'
                          }
                          size='sm'
                          copyable={false}
                        />
                      ),
                    },
                    {
                      id: 'remark',
                      header: t('Remark'),
                      cell: (environment) => environment.remark || '-',
                    },
                    {
                      id: 'created',
                      header: t('Created at'),
                      cell: (environment) =>
                        environment.created_time
                          ? formatTimestampToDate(environment.created_time)
                          : '-',
                    },
                    {
                      id: 'actions',
                      header: t('Actions'),
                      cell: (environment) => (
                        <div className='flex gap-1'>
                          <Button
                            variant='outline'
                            size='sm'
                            disabled={testingId === environment.id}
                            onClick={() => void handleTest(environment)}
                          >
                            <Plug className='h-4 w-4' />
                            {t('Test connection')}
                          </Button>
                          <Button
                            variant='outline'
                            size='sm'
                            onClick={() =>
                              setMutateState({ currentRow: environment })
                            }
                          >
                            {t('Edit')}
                          </Button>
                          <Button
                            variant='ghost'
                            size='icon'
                            className='text-destructive'
                            aria-label={t('Delete')}
                            onClick={() => setDeleteTarget(environment)}
                          >
                            <Trash2 className='h-4 w-4' />
                          </Button>
                        </div>
                      ),
                    },
                  ]}
                />
              )}
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      {mutateState !== null && (
        <SyncEnvironmentMutateDialog
          open
          onOpenChange={(open) => !open && setMutateState(null)}
          currentRow={mutateState.currentRow}
          onSaved={() => void refresh()}
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t('Delete environment')}
        desc={`${t('Confirm deletion')}: ${deleteTarget?.name ?? ''}`}
        destructive
        handleConfirm={() => {
          if (deleteTarget) void handleDelete(deleteTarget)
        }}
      />
    </>
  )
}

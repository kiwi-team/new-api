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
import { Edit, Plus, RefreshCw, Search, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useDebounce } from '@/hooks/use-debounce'

import {
  deleteModelRouteConfig,
  getChannelNameList,
  getModelRouteConfigs,
  updateModelRouteConfigStatus,
} from './api'
import {
  ChannelGroupsCell,
  PatternBadgeList,
} from './components/route-config-cells'
import { RouteConfigMutateDialog } from './components/route-config-mutate-dialog'
import { SpecialChannelsTab } from './components/special-channels-tab'
import type { ModelRouteConfig } from './types'

const PAGE_SIZE = 20

const route = getRouteApi('/_authenticated/model-route-config/')

export function ModelRouteConfigPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const navigate = route.useNavigate()
  const activeTab = route.useSearch().tab ?? 'routes'
  const [keyword, setKeyword] = useState('')
  const [modelKeyword, setModelKeyword] = useState('')
  const debouncedKeyword = useDebounce(keyword, 300)
  const debouncedModelKeyword = useDebounce(modelKeyword, 300)
  const [mutateState, setMutateState] = useState<{
    currentRow?: ModelRouteConfig
  } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ModelRouteConfig | null>(null)

  const configsQuery = useQuery({
    queryKey: [
      'model-route-config',
      'list',
      debouncedKeyword,
      debouncedModelKeyword,
    ],
    queryFn: async () => {
      const result = await getModelRouteConfigs({
        p: 1,
        page_size: PAGE_SIZE,
        keyword: debouncedKeyword,
        model_keyword: debouncedModelKeyword,
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
    placeholderData: (previous) => previous,
  })

  const channelsQuery = useQuery({
    queryKey: ['model-route-config', 'channels'],
    queryFn: getChannelNameList,
  })

  // Channel ids are stored on the config; names come from a separate lookup.
  const channelNames = useMemo(() => {
    const map: Record<number, string> = {}
    for (const channel of channelsQuery.data ?? []) {
      map[channel.id] = channel.name
    }
    return map
  }, [channelsQuery.data])

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['model-route-config', 'list'] })

  const handleToggleStatus = async (config: ModelRouteConfig) => {
    const res = await updateModelRouteConfigStatus(
      config.id,
      config.enabled === 1 ? 0 : 1
    )
    if (!res.success) {
      toast.error(res.message || t('Operation failed'))
      return
    }
    toast.success(t('Status updated'))
    await refresh()
  }

  const handleDelete = async (config: ModelRouteConfig) => {
    setDeleteTarget(null)
    const res = await deleteModelRouteConfig(config.id)
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
        <SectionPageLayout.Title>
          {t('Model Route Config')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          {activeTab === 'routes' && (
            <Button size='sm' onClick={() => setMutateState({})}>
              <Plus className='h-4 w-4' />
              {t('Create')}
            </Button>
          )}
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex h-full min-h-0 flex-col gap-4'>
            <Tabs
              value={activeTab}
              onValueChange={(tab) =>
                void navigate({ search: { tab: tab as 'routes' | 'special' } })
              }
            >
              <TabsList>
                <TabsTrigger value='routes'>{t('Route rules')}</TabsTrigger>
                <TabsTrigger value='special'>
                  {t('Special channels')}
                </TabsTrigger>
              </TabsList>
            </Tabs>

            {activeTab === 'special' && <SpecialChannelsTab />}
            {activeTab === 'routes' && (
            <div className='flex h-full min-h-0 flex-col gap-4'>
            <div className='flex flex-wrap items-center gap-2'>
              <div className='relative'>
                <Search className='text-muted-foreground pointer-events-none absolute inset-y-0 start-2 my-auto h-4 w-4' />
                <Input
                  className='w-56 ps-8'
                  placeholder={t('Search by config name')}
                  value={keyword}
                  onChange={(event) => setKeyword(event.target.value)}
                />
              </div>
              <Input
                className='w-56'
                placeholder={t('Search by model keyword')}
                value={modelKeyword}
                onChange={(event) => setModelKeyword(event.target.value)}
              />
              <Button
                variant='outline'
                size='sm'
                onClick={() => void refresh()}
                disabled={configsQuery.isFetching}
              >
                <RefreshCw className='h-4 w-4' />
                {t('Refresh')}
              </Button>
            </div>

            <div className='min-h-0 flex-1 overflow-auto'>
              <StaticDataTable
                tableClassName='min-w-max'
                data={configsQuery.data?.items ?? []}
                getRowKey={(config) => config.id}
                emptyContent={t('No route configs yet')}
                emptyClassName='text-muted-foreground py-8'
                columns={[
                  { id: 'id', header: t('ID'), cell: (config) => config.id },
                  {
                    id: 'name',
                    header: t('Config name'),
                    cellClassName: 'font-medium',
                    cell: (config) => (
                      <span className='flex items-center gap-2'>
                        {config.name}
                        {config.enabled === 0 && (
                          <Badge variant='destructive'>{t('Disabled')}</Badge>
                        )}
                      </span>
                    ),
                  },
                  {
                    id: 'rules',
                    header: t('Match rules'),
                    cell: (config) => {
                      const hasRules =
                        config.model_patterns?.length ||
                        config.body_patterns?.length ||
                        config.url_patterns?.length
                      if (!hasRules) {
                        return <span className='text-muted-foreground'>-</span>
                      }
                      return (
                        <div className='flex flex-wrap items-center gap-1.5'>
                          <PatternBadgeList
                            label={t('Model')}
                            items={config.model_patterns ?? []}
                          />
                          <PatternBadgeList
                            label={t('Body')}
                            items={config.body_patterns ?? []}
                          />
                          <PatternBadgeList
                            label={t('URL')}
                            items={config.url_patterns ?? []}
                          />
                        </div>
                      )
                    },
                  },
                  {
                    id: 'channel-groups',
                    header: t('Channel groups'),
                    cell: (config) => (
                      <ChannelGroupsCell
                        groups={config.channel_groups ?? []}
                        channelNames={channelNames}
                      />
                    ),
                  },
                  {
                    id: 'random-type',
                    header: t('Selection mode'),
                    cell: (config) => (
                      <Badge variant='outline'>
                        {config.random_type === 'random'
                          ? t('Random')
                          : t('In order')}
                      </Badge>
                    ),
                  },
                  {
                    id: 'priority',
                    header: t('Priority'),
                    cell: (config) => config.priority,
                  },
                  {
                    id: 'max-retry',
                    header: t('Max retries'),
                    cell: (config) => config.max_retry,
                  },
                  {
                    id: 'enabled',
                    header: t('Status'),
                    cell: (config) => (
                      <Switch
                        checked={config.enabled === 1}
                        onCheckedChange={() =>
                          void handleToggleStatus(config)
                        }
                        aria-label={t('Status')}
                      />
                    ),
                  },
                  {
                    id: 'actions',
                    header: t('Actions'),
                    cell: (config) => (
                      <div className='flex gap-1'>
                        <Button
                          variant='outline'
                          size='sm'
                          onClick={() => setMutateState({ currentRow: config })}
                        >
                          <Edit className='h-4 w-4' />
                          {t('Edit')}
                        </Button>
                        <Button
                          variant='ghost'
                          size='icon'
                          className='text-destructive'
                          aria-label={t('Delete')}
                          onClick={() => setDeleteTarget(config)}
                        >
                          <Trash2 className='h-4 w-4' />
                        </Button>
                      </div>
                    ),
                  },
                ]}
              />
            </div>
            </div>
            )}
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      {mutateState !== null && (
        <RouteConfigMutateDialog
          open
          onOpenChange={(open) => !open && setMutateState(null)}
          currentRow={mutateState.currentRow}
          onSaved={() => void refresh()}
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t('Delete route config')}
        desc={`${t('Confirm deletion')}: ${deleteTarget?.name ?? ''}`}
        destructive
        handleConfirm={() => {
          if (deleteTarget) void handleDelete(deleteTarget)
        }}
      />
    </>
  )
}

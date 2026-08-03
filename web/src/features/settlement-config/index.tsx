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
import { Copy, Plus, Trash2, Upload } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useDebounce } from '@/hooks/use-debounce'

import {
  deleteSettlementConfig,
  getSettlementConfigs,
  searchUserOptions,
} from './api'
import { BatchImportDialog } from './components/batch-import-dialog'
import { SettlementConfigMutateDialog } from './components/settlement-config-mutate-dialog'
import type { SettlementConfig } from './types'

const route = getRouteApi('/_authenticated/settlement-config/')

function formatPrice(value: number | null | undefined): string {
  return value === null || value === undefined ? '0' : Number(value).toFixed(4)
}

type MutateState = {
  currentRow?: SettlementConfig
  seed?: Partial<SettlementConfig>
}

export function SettlementConfigPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const userId = search.userId
  const [userKeyword, setUserKeyword] = useState('')
  const debouncedKeyword = useDebounce(userKeyword, 300)
  const [mutateState, setMutateState] = useState<MutateState | null>(null)
  const [batchOpen, setBatchOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<SettlementConfig | null>(null)

  const userOptionsQuery = useQuery({
    queryKey: ['settlement-config', 'user-options', debouncedKeyword],
    queryFn: () => searchUserOptions(debouncedKeyword),
  })

  // The list is per customer, so nothing is fetched until one is picked.
  const configsQuery = useQuery({
    queryKey: ['settlement-config', 'list', userId],
    queryFn: () => getSettlementConfigs(userId as number),
    enabled: userId !== undefined,
  })

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['settlement-config', 'list'] })

  const selectUser = (nextUserId?: number) =>
    void navigate({ search: { userId: nextUserId } })

  const handleDelete = async (config: SettlementConfig) => {
    setDeleteTarget(null)
    const res = await deleteSettlementConfig(config.id)
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
        {t('Settlement Prices')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <div className='flex flex-wrap items-center gap-2'>
          <Select
            items={userOptionsQuery.data?.map((option) => ({
              value: String(option.value),
              label: option.label,
            }))}
            value={userId === undefined ? '' : String(userId)}
            onValueChange={(value) =>
              selectUser(value ? Number.parseInt(value, 10) : undefined)
            }
          >
            <SelectTrigger className='w-60'>
              <SelectValue placeholder={t('Search users')} />
            </SelectTrigger>
            <SelectContent>
              <div className='p-1'>
                <Input
                  placeholder={t('Search users')}
                  value={userKeyword}
                  onChange={(event) => setUserKeyword(event.target.value)}
                />
              </div>
              {(userOptionsQuery.data ?? []).map((option) => (
                <SelectItem key={option.value} value={String(option.value)}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            size='sm'
            onClick={() => setMutateState({ seed: { user_id: userId } })}
          >
            <Plus className='h-4 w-4' />
            {t('Add')}
          </Button>
          <Button variant='outline' size='sm' onClick={() => setBatchOpen(true)}>
            <Upload className='h-4 w-4' />
            {t('Batch import')}
          </Button>
        </div>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='min-h-0 flex-1 overflow-auto'>
          <StaticDataTable
            tableClassName='min-w-max'
            data={configsQuery.data ?? []}
            getRowKey={(config) => config.id}
            emptyContent={
              userId === undefined
                ? t('Pick a customer to view their settlement prices')
                : t('No settlement prices configured')
            }
            emptyClassName='text-muted-foreground py-8'
            columns={[
              { id: 'id', header: 'ID', cell: (config) => config.id },
              {
                id: 'user-id',
                header: t('User ID'),
                cell: (config) => config.user_id,
              },
              {
                id: 'model',
                header: t('Model name'),
                cellClassName: 'font-medium',
                cell: (config) => config.model_name,
              },
              {
                id: 'discount',
                header: t('Model discount'),
                cell: (config) => Number(config.discount ?? 1).toFixed(2),
              },
              {
                id: 'input-price',
                header: `${t('Input price')} ($/1M tokens)`,
                cell: (config) => formatPrice(config.input_price),
              },
              {
                id: 'output-price',
                header: `${t('Output price')} ($/1M tokens)`,
                cell: (config) => formatPrice(config.output_price),
              },
              {
                id: 'request-price',
                header: `${t('Per-call price')} ($)`,
                cell: (config) => formatPrice(config.request_price),
              },
              {
                id: 'created-at',
                header: t('Created at'),
                cell: (config) =>
                  config.created_at
                    ? new Date(config.created_at * 1000).toLocaleString()
                    : '-',
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
                      {t('Edit')}
                    </Button>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() =>
                        setMutateState({
                          seed: {
                            ...config,
                            model_name: `${config.model_name}_copy`,
                          },
                        })
                      }
                    >
                      <Copy className='h-4 w-4' />
                      {t('Copy')}
                    </Button>
                    <Button
                      variant='ghost'
                      size='icon'
                      className='text-destructive'
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
      </SectionPageLayout.Content>
    </SectionPageLayout>

      {mutateState !== null && (
        <SettlementConfigMutateDialog
          open
          onOpenChange={(open) => !open && setMutateState(null)}
          currentRow={mutateState.currentRow}
          seed={mutateState.seed}
          onSaved={() => void refresh()}
        />
      )}

      <BatchImportDialog
        open={batchOpen}
        onOpenChange={setBatchOpen}
        onImported={(importedUserId) => {
          if (importedUserId !== undefined) selectUser(importedUserId)
          void refresh()
        }}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t('Delete settlement price')}
        desc={`${t('Confirm deletion')}: ${deleteTarget?.model_name ?? ''}`}
        destructive
        handleConfirm={() => {
          if (deleteTarget) void handleDelete(deleteTarget)
        }}
      />
    </>
  )
}

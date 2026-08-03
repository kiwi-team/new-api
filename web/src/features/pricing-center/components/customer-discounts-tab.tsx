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
import {
  ChevronDown,
  ChevronRight,
  Plus,
  RefreshCw,
  Search,
  Trash2,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import {
  deleteSettlementConfig,
  getAllSettlementConfigs,
} from '@/features/settlement-config/api'
import { SettlementConfigMutateDialog } from '@/features/settlement-config/components/settlement-config-mutate-dialog'
import type { SettlementConfig } from '@/features/settlement-config/types'

import { getPricingOptions } from '../api'
import { buildOfficialPriceMap, formatPrice } from '../lib'

type ModelGroup = {
  model_name: string
  customerCount: number
  official: { input: number | null; output: number | null } | null
  configs: SettlementConfig[]
}

type MutateState = {
  currentRow?: SettlementConfig
  seed?: Partial<SettlementConfig>
}

export function CustomerDiscountsTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [keyword, setKeyword] = useState('')
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})
  const [mutateState, setMutateState] = useState<MutateState | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<SettlementConfig | null>(null)

  const configsQuery = useQuery({
    queryKey: ['pricing-center', 'settlement-configs'],
    queryFn: getAllSettlementConfigs,
  })
  const optionsQuery = useQuery({
    queryKey: ['pricing-center', 'options'],
    queryFn: getPricingOptions,
  })

  const officialPrices = useMemo(
    () => buildOfficialPriceMap(optionsQuery.data ?? {}),
    [optionsQuery.data]
  )

  const groups = useMemo<ModelGroup[]>(() => {
    const byModel = new Map<string, SettlementConfig[]>()
    for (const config of configsQuery.data ?? []) {
      const list = byModel.get(config.model_name) ?? []
      list.push(config)
      byModel.set(config.model_name, list)
    }
    const kw = keyword.trim().toLowerCase()
    return [...byModel.entries()]
      .filter(([model]) => !kw || model.toLowerCase().includes(kw))
      .map(([model, configs]) => ({
        model_name: model,
        customerCount: configs.length,
        official: officialPrices[model] ?? null,
        configs,
      }))
  }, [configsQuery.data, keyword, officialPrices])

  const refresh = () =>
    queryClient.invalidateQueries({
      queryKey: ['pricing-center', 'settlement-configs'],
    })

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
    <div className='flex h-full min-h-0 flex-col gap-4'>
      <div className='flex flex-wrap items-center gap-2'>
        <div className='relative'>
          <Search className='text-muted-foreground pointer-events-none absolute inset-y-0 start-2 my-auto h-4 w-4' />
          <Input
            className='w-56 ps-8'
            placeholder={t('Filter by model keyword')}
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
          />
        </div>
        <Button
          variant='outline'
          size='sm'
          onClick={() => void refresh()}
          disabled={configsQuery.isFetching}
        >
          <RefreshCw className='h-4 w-4' />
          {t('Refresh')}
        </Button>
        <Button size='sm' onClick={() => setMutateState({})}>
          <Plus className='h-4 w-4' />
          {t('Add customer discount')}
        </Button>
      </div>

      <div className='min-h-0 flex-1 overflow-auto'>
        <StaticDataTable
          data={groups}
          getRowKey={(group) => group.model_name}
          emptyContent={t('No customer discounts configured yet')}
          emptyClassName='text-muted-foreground py-8'
          renderRow={(group) => (
            <ModelGroupRows
              key={group.model_name}
              group={group}
              expanded={expanded[group.model_name] === true}
              onToggle={() =>
                setExpanded((prev) => ({
                  ...prev,
                  [group.model_name]: !prev[group.model_name],
                }))
              }
              onAddForModel={() =>
                setMutateState({ seed: { model_name: group.model_name } })
              }
              onEdit={(config) => setMutateState({ currentRow: config })}
              onDelete={(config) => setDeleteTarget(config)}
            />
          )}
          columns={[
            { id: 'model', header: t('Model name') },
            { id: 'official-input', header: `${t('List input price')} ($/1M)` },
            { id: 'official-output', header: `${t('List output price')} ($/1M)` },
            { id: 'customers', header: t('Customers') },
            { id: 'actions', header: t('Actions') },
          ]}
        />
      </div>

      {mutateState !== null && (
        <SettlementConfigMutateDialog
          open
          onOpenChange={(open) => !open && setMutateState(null)}
          currentRow={mutateState.currentRow}
          seed={mutateState.seed}
          onSaved={() => void refresh()}
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t('Delete customer discount')}
        desc={`${t('Confirm deletion')}: ${deleteTarget?.model_name ?? ''}`}
        destructive
        handleConfirm={() => {
          if (deleteTarget) void handleDelete(deleteTarget)
        }}
      />
    </div>
  )
}

type ModelGroupRowsProps = {
  group: ModelGroup
  expanded: boolean
  onToggle: () => void
  onAddForModel: () => void
  onEdit: (config: SettlementConfig) => void
  onDelete: (config: SettlementConfig) => void
}

/**
 * One model row plus, when expanded, the per-customer overrides beneath it.
 * Rendered as sibling rows so the group and its children share column widths.
 */
function ModelGroupRows({
  group,
  expanded,
  onToggle,
  onAddForModel,
  onEdit,
  onDelete,
}: ModelGroupRowsProps) {
  const { t } = useTranslation()

  return (
    <>
      <tr className='hover:bg-muted/50 border-b transition-colors'>
        <td className='p-2 align-middle font-medium'>
          <button
            type='button'
            className='flex items-center gap-1'
            onClick={onToggle}
            aria-expanded={expanded}
          >
            {expanded ? (
              <ChevronDown className='h-4 w-4' />
            ) : (
              <ChevronRight className='h-4 w-4' />
            )}
            {group.model_name}
          </button>
        </td>
        <td className='p-2 align-middle'>{formatPrice(group.official?.input)}</td>
        <td className='p-2 align-middle'>
          {formatPrice(group.official?.output)}
        </td>
        <td className='p-2 align-middle'>
          <Badge variant='secondary'>{group.customerCount}</Badge>
        </td>
        <td className='p-2 align-middle'>
          <Button variant='outline' size='sm' onClick={onAddForModel}>
            <Plus className='h-4 w-4' />
            {t('Add discount for this model')}
          </Button>
        </td>
      </tr>
      {expanded &&
        group.configs.map((config) => (
          <tr key={config.id} className='bg-muted/30 border-b text-sm'>
            <td className='py-2 ps-8 align-middle'>
              {config.username || '-'} (ID: {config.user_id})
            </td>
            <td className='p-2 align-middle'>
              {t('Discount')} {Number(config.discount ?? 1).toFixed(2)} ·{' '}
              {t('Input')} {formatPrice(config.input_price)}
            </td>
            <td className='p-2 align-middle'>
              {t('Output')} {formatPrice(config.output_price)}
            </td>
            <td className='p-2 align-middle'>
              {t('Per-call')} {formatPrice(config.request_price)}
            </td>
            <td className='flex gap-1 p-2 align-middle'>
              <Button variant='outline' size='sm' onClick={() => onEdit(config)}>
                {t('Edit')}
              </Button>
              <Button
                variant='ghost'
                size='icon'
                className='text-destructive'
                onClick={() => onDelete(config)}
              >
                <Trash2 className='h-4 w-4' />
              </Button>
            </td>
          </tr>
        ))}
    </>
  )
}

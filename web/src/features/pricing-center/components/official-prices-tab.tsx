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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, RefreshCw, Search, Trash2, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import {
  deleteModelPricing,
  getExchangeRate,
  getPricingOptions,
  saveTieredPriceMap,
  updateModelPricing,
} from '../api'
import {
  BILLING_MODES,
  DEFAULT_EXCHANGE_RATE,
  DEFAULT_FIRST_TIER_MAX_TOKENS,
  getBillingModeLabel,
} from '../constants'
import {
  buildOfficialPriceRows,
  formatTokens,
  parseTieredPriceMap,
} from '../lib'
import type { BillingMode, OfficialPriceRow, PriceTier } from '../types'
import { DualPriceInput } from './dual-price-input'
import { TierEditorDialog } from './tier-editor-dialog'

type TierEditorState = {
  model: string
  tierIndex: number
}

export function OfficialPricesTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [keyword, setKeyword] = useState('')
  const [newModelName, setNewModelName] = useState('')
  const [newBillingMode, setNewBillingMode] = useState<BillingMode>('token')
  const [tierEditor, setTierEditor] = useState<TierEditorState | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<OfficialPriceRow | null>(null)

  const optionsQuery = useQuery({
    queryKey: ['pricing-center', 'options'],
    queryFn: getPricingOptions,
  })
  const rateQuery = useQuery({
    queryKey: ['pricing-center', 'exchange-rate'],
    queryFn: getExchangeRate,
    staleTime: 30 * 60 * 1000,
  })
  const rate = rateQuery.data ?? DEFAULT_EXCHANGE_RATE

  const options = useMemo(() => optionsQuery.data ?? {}, [optionsQuery.data])
  const rows = useMemo(() => buildOfficialPriceRows(options), [options])
  const visibleRows = useMemo(() => {
    const kw = keyword.trim().toLowerCase()
    if (!kw) return rows
    return rows.filter((row) => row.model.toLowerCase().includes(kw))
  }, [keyword, rows])

  // `SelectValue` resolves its label from `items`, so the trigger shows the
  // billing mode name instead of the raw value.
  const billingModeItems = useMemo(
    () =>
      BILLING_MODES.map((mode) => ({
        value: mode,
        label: getBillingModeLabel(mode, t),
      })),
    [t]
  )

  const invalidateOptions = () =>
    queryClient.invalidateQueries({ queryKey: ['pricing-center', 'options'] })

  const savePrice = useMutation({
    mutationFn: async (input: {
      row: Pick<
        OfficialPriceRow,
        | 'model'
        | 'billingMode'
        | 'inputUSD'
        | 'outputUSD'
        | 'perCallUSD'
        | 'cacheReadUSD'
        | 'cacheCreateUSD'
      >
      patch?: Partial<OfficialPriceRow>
    }) => {
      const merged = { ...input.row, ...input.patch }
      const res = await updateModelPricing({
        model_name: merged.model,
        is_per_call: merged.billingMode === 'call',
        input_price: merged.inputUSD || 0,
        output_price: merged.outputUSD || 0,
        per_call_price: merged.perCallUSD || 0,
        cache_read_price: merged.cacheReadUSD || 0,
        cache_create_price: merged.cacheCreateUSD || 0,
      })
      if (!res.success) throw new Error(res.message || t('Operation failed'))
    },
    onSuccess: async () => {
      toast.success(t('Saved'))
      await invalidateOptions()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const saveTiers = useMutation({
    mutationFn: async (input: { model: string; tiers: PriceTier[] }) => {
      const map = parseTieredPriceMap(options)
      if (input.tiers.length > 0) {
        map[input.model] = [...input.tiers].sort(
          (a, b) => a.max_tokens - b.max_tokens
        )
      } else {
        delete map[input.model]
      }
      const res = await saveTieredPriceMap(map)
      if (!res.success) throw new Error(res.message || t('Operation failed'))
    },
    onSuccess: async () => {
      toast.success(t('Saved'))
      await invalidateOptions()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const isSaving = savePrice.isPending || saveTiers.isPending

  /**
   * Billing modes are mutually exclusive in storage, so switching clears the
   * other shape first — otherwise a model would carry both a ratio and a tier
   * list and the reader would pick the wrong one.
   */
  const switchBillingMode = async (row: OfficialPriceRow, mode: BillingMode) => {
    if (mode === row.billingMode) return
    if (mode === 'tiered') {
      await deleteModelPricing(row.model)
      await saveTiers.mutateAsync({
        model: row.model,
        tiers: row.tiers.length
          ? row.tiers
          : [
              {
                max_tokens: DEFAULT_FIRST_TIER_MAX_TOKENS,
                input_price: 0,
                output_price: 0,
              },
            ],
      })
      return
    }
    if (row.billingMode === 'tiered') {
      await saveTiers.mutateAsync({ model: row.model, tiers: [] })
      await savePrice.mutateAsync({
        row: {
          model: row.model,
          billingMode: mode,
          inputUSD: 0,
          outputUSD: 0,
          perCallUSD: 0,
          cacheReadUSD: 0,
          cacheCreateUSD: 0,
        },
      })
      return
    }
    await savePrice.mutateAsync({ row, patch: { billingMode: mode } })
  }

  const handleAddModel = async () => {
    const name = newModelName.trim()
    if (!name) {
      toast.error(t('Model name is required'))
      return
    }
    if (newBillingMode === 'tiered') {
      await saveTiers.mutateAsync({
        model: name,
        tiers: [
          {
            max_tokens: DEFAULT_FIRST_TIER_MAX_TOKENS,
            input_price: 0,
            output_price: 0,
          },
        ],
      })
    } else {
      await savePrice.mutateAsync({
        row: {
          model: name,
          billingMode: newBillingMode,
          inputUSD: 0,
          outputUSD: 0,
          perCallUSD: 0,
          cacheReadUSD: 0,
          cacheCreateUSD: 0,
        },
      })
    }
    setNewModelName('')
  }

  const handleDeleteModel = async (row: OfficialPriceRow) => {
    setDeleteTarget(null)
    if (row.billingMode === 'tiered') {
      await saveTiers.mutateAsync({ model: row.model, tiers: [] })
      return
    }
    const res = await deleteModelPricing(row.model)
    if (res.success) {
      toast.success(t('Deleted successfully'))
      await invalidateOptions()
      return
    }
    toast.error(res.message || t('Operation failed'))
  }

  const editorRow = tierEditor
    ? rows.find((row) => row.model === tierEditor.model)
    : undefined

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
          onClick={() => void invalidateOptions()}
          disabled={optionsQuery.isFetching}
        >
          <RefreshCw className='h-4 w-4' />
          {t('Refresh')}
        </Button>
        <Badge variant='outline'>
          {t('Current exchange rate')} 1$ = {rate}¥
        </Badge>
      </div>

      <div className='bg-muted/40 flex flex-wrap items-center gap-2 rounded-lg p-3'>
        <span className='text-muted-foreground text-sm'>{t('Add model')}</span>
        <Input
          className='w-60'
          placeholder={t('Model name')}
          value={newModelName}
          onChange={(event) => setNewModelName(event.target.value)}
        />
        <Select
          items={billingModeItems}
          value={newBillingMode}
          onValueChange={(value) => setNewBillingMode(value as BillingMode)}
        >
          <SelectTrigger className='w-36'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {BILLING_MODES.map((mode) => (
              <SelectItem key={mode} value={mode}>
                {getBillingModeLabel(mode, t)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button size='sm' onClick={() => void handleAddModel()} disabled={isSaving}>
          <Plus className='h-4 w-4' />
          {t('Add')}
        </Button>
        <span className='text-muted-foreground text-xs'>
          {t('Prices can be edited inline after adding; changes apply immediately')}
        </span>
      </div>

      <div className='min-h-0 flex-1 overflow-auto'>
        <StaticDataTable
          tableClassName='min-w-max'
          data={visibleRows}
          getRowKey={(row) => row.model}
          emptyContent={t('No prices configured yet')}
          emptyClassName='text-muted-foreground py-8'
          columns={[
            {
              id: 'model',
              header: t('Model name'),
              cellClassName: 'font-medium',
              cell: (row) => row.model,
            },
            {
              id: 'billing-mode',
              header: t('Billing mode'),
              cell: (row) => (
                <Select
                  items={billingModeItems}
                  value={row.billingMode}
                  onValueChange={(value) =>
                    void switchBillingMode(row, value as BillingMode)
                  }
                >
                  <SelectTrigger className='h-8 w-32 text-xs'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {BILLING_MODES.map((mode) => (
                      <SelectItem key={mode} value={mode}>
                        {getBillingModeLabel(mode, t)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ),
            },
            {
              id: 'input-price',
              header: `${t('Input price')} ($/¥ /1M)`,
              cell: (row) =>
                row.billingMode === 'tiered' ? (
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() =>
                      setTierEditor({ model: row.model, tierIndex: -1 })
                    }
                  >
                    {t('Edit tiers')} ({row.tiers.length})
                  </Button>
                ) : (
                  <DualPriceInput
                    value={row.inputUSD}
                    rate={rate}
                    disabled={row.billingMode !== 'token'}
                    onCommit={(usd) =>
                      savePrice.mutate({ row, patch: { inputUSD: usd } })
                    }
                  />
                ),
            },
            {
              id: 'output-price',
              header: `${t('Output price')} ($/¥ /1M)`,
              cell: (row) =>
                row.billingMode === 'tiered' ? (
                  <div className='flex flex-wrap items-center gap-1'>
                    {row.tiers.map((tier, index) => (
                      <Badge
                        key={tier.max_tokens}
                        variant='secondary'
                        className='cursor-pointer gap-1'
                        onClick={() =>
                          setTierEditor({ model: row.model, tierIndex: index })
                        }
                      >
                        ≤{formatTokens(tier.max_tokens)}: ${tier.input_price}/$
                        {tier.output_price}
                        {tier.cached_input_price
                          ? ` ${t('Cache read')} $${tier.cached_input_price}`
                          : ''}
                        {tier.cache_write_price
                          ? ` ${t('Cache write')} $${tier.cache_write_price}`
                          : ''}
                        <X
                          className='h-3 w-3'
                          onClick={(event) => {
                            event.stopPropagation()
                            saveTiers.mutate({
                              model: row.model,
                              tiers: row.tiers.filter((_, i) => i !== index),
                            })
                          }}
                        />
                      </Badge>
                    ))}
                    <Button
                      variant='ghost'
                      size='icon'
                      className='h-6 w-6'
                      onClick={() =>
                        setTierEditor({ model: row.model, tierIndex: -1 })
                      }
                    >
                      <Plus className='h-3 w-3' />
                    </Button>
                  </div>
                ) : (
                  <DualPriceInput
                    value={row.outputUSD}
                    rate={rate}
                    disabled={row.billingMode !== 'token'}
                    onCommit={(usd) =>
                      savePrice.mutate({ row, patch: { outputUSD: usd } })
                    }
                  />
                ),
            },
            {
              id: 'cache-read-price',
              header: `${t('Cache read price')} ($/¥ /1M)`,
              cell: (row) => (
                <DualPriceInput
                  value={row.cacheReadUSD}
                  rate={rate}
                  disabled={row.billingMode !== 'token'}
                  onCommit={(usd) =>
                    savePrice.mutate({ row, patch: { cacheReadUSD: usd } })
                  }
                />
              ),
            },
            {
              id: 'cache-create-price',
              header: `${t('Cache write price')} ($/¥ /1M)`,
              cell: (row) => (
                <DualPriceInput
                  value={row.cacheCreateUSD}
                  rate={rate}
                  disabled={row.billingMode !== 'token'}
                  onCommit={(usd) =>
                    savePrice.mutate({ row, patch: { cacheCreateUSD: usd } })
                  }
                />
              ),
            },
            {
              id: 'per-call-price',
              header: `${t('Per-call price')} ($/¥)`,
              cell: (row) => (
                <DualPriceInput
                  value={row.perCallUSD}
                  rate={rate}
                  disabled={row.billingMode !== 'call'}
                  onCommit={(usd) =>
                    savePrice.mutate({ row, patch: { perCallUSD: usd } })
                  }
                />
              ),
            },
            {
              id: 'updated-at',
              header: t('Updated at'),
              cell: (row) =>
                row.updatedAt
                  ? new Date(row.updatedAt * 1000).toLocaleString()
                  : '-',
            },
            {
              id: 'actions',
              header: t('Actions'),
              cell: (row) => (
                <Button
                  variant='ghost'
                  size='icon'
                  className='text-destructive'
                  onClick={() => setDeleteTarget(row)}
                >
                  <Trash2 className='h-4 w-4' />
                </Button>
              ),
            },
          ]}
        />
      </div>

      {tierEditor && editorRow && (
        <TierEditorDialog
          open
          onOpenChange={(open) => !open && setTierEditor(null)}
          modelName={tierEditor.model}
          tier={
            tierEditor.tierIndex >= 0
              ? editorRow.tiers[tierEditor.tierIndex]
              : undefined
          }
          usedMaxTokens={editorRow.tiers.map((tier) => tier.max_tokens)}
          onSave={(tier) => {
            const tiers = [...editorRow.tiers]
            if (tierEditor.tierIndex >= 0) tiers[tierEditor.tierIndex] = tier
            else tiers.push(tier)
            saveTiers.mutate({ model: editorRow.model, tiers })
          }}
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t('Delete price')}
        desc={`${t('Confirm deletion')}: ${deleteTarget?.model ?? ''}`}
        destructive
        handleConfirm={() => {
          if (deleteTarget) void handleDeleteModel(deleteTarget)
        }}
      />
    </div>
  )
}

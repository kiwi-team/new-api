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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'

import {
  createUsageAdjustment,
  getUsageHourSnapshot,
  revertUsageAdjustment,
} from './api'
import { formatCount, formatUsd, usageHourTimestamp } from './lib'
import type { ModelUsageRow, UsageHourValues } from './types'

type CorrectedValues = {
  input_tokens: string
  output_tokens: string
  cache_read_tokens: string
  cache_write_5m_tokens: string
  cache_write_1h_tokens: string
  cost_usd: string
}

const EMPTY_VALUES: CorrectedValues = {
  input_tokens: '0',
  output_tokens: '0',
  cache_read_tokens: '0',
  cache_write_5m_tokens: '0',
  cache_write_1h_tokens: '0',
  cost_usd: '0',
}

function valuesForForm(values: UsageHourValues): CorrectedValues {
  return {
    input_tokens: String(values.input_tokens),
    output_tokens: String(values.output_tokens),
    cache_read_tokens: String(values.cache_read_tokens),
    cache_write_5m_tokens: String(values.cache_write_5m_tokens),
    cache_write_1h_tokens: String(values.cache_write_1h_tokens),
    cost_usd: String(values.cost_usd),
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Operation failed'
}

type UsageAdjustmentDialogProps = {
  open: boolean
  row: ModelUsageRow | null
  onOpenChange: (open: boolean) => void
  onSaved: () => void
}

export function UsageAdjustmentDialog(props: UsageAdjustmentDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [hour, setHour] = useState(0)
  const [values, setValues] = useState<CorrectedValues>(EMPTY_VALUES)
  const [reason, setReason] = useState('')
  const [ticket, setTicket] = useState('')
  const [revertReason, setRevertReason] = useState('')

  const hourStart = props.row ? usageHourTimestamp(props.row.date, hour) : 0
  const snapshotQuery = useQuery({
    queryKey: [
      'model-usage-adjustment-hour',
      hourStart,
      props.row?.token_id,
      props.row?.model_name,
    ],
    queryFn: () =>
      getUsageHourSnapshot({
        hour_start: hourStart,
        token_id: props.row?.token_id ?? 0,
        model_name: props.row?.model_name ?? '',
      }),
    enabled:
      props.open &&
      hourStart > 0 &&
      !!props.row?.token_id &&
      !!props.row?.model_name,
    retry: false,
  })

  useEffect(() => {
    if (!props.open) return
    setReason('')
    setTicket('')
    setRevertReason('')
  }, [props.open, props.row])

  useEffect(() => {
    if (snapshotQuery.data) {
      setValues(valuesForForm(snapshotQuery.data.effective))
    }
  }, [snapshotQuery.data])

  const createMutation = useMutation({
    mutationFn: createUsageAdjustment,
    onSuccess: (snapshot) => {
      queryClient.setQueryData(
        [
          'model-usage-adjustment-hour',
          snapshot.hour_start,
          snapshot.token_id,
          snapshot.model_name,
        ],
        snapshot
      )
      setValues(valuesForForm(snapshot.effective))
      setReason('')
      setTicket('')
      props.onSaved()
      toast.success(t('Usage correction saved'))
    },
    onError: (error) => toast.error(t(errorMessage(error))),
  })

  const revertMutation = useMutation({
    mutationFn: (id: number) => revertUsageAdjustment(id, revertReason),
    onSuccess: async () => {
      setRevertReason('')
      await snapshotQuery.refetch()
      props.onSaved()
      toast.success(t('Usage correction reverted'))
    },
    onError: (error) => toast.error(t(errorMessage(error))),
  })

  const updateValue = (field: keyof CorrectedValues, value: string) => {
    setValues((current) => ({ ...current, [field]: value }))
  }

  const handleSubmit = () => {
    if (!props.row?.token_id || !props.row.model_name || !reason.trim()) {
      toast.warning(t('A correction reason is required'))
      return
    }
    const parsed = Object.fromEntries(
      Object.entries(values).map(([key, value]) => [key, Number(value)])
    ) as Record<keyof CorrectedValues, number>
    if (
      Object.values(parsed).some(
        (value) => !Number.isFinite(value) || value < 0
      )
    ) {
      toast.warning(t('Corrected values must be non-negative numbers'))
      return
    }
    createMutation.mutate({
      hour_start: hourStart,
      token_id: props.row.token_id,
      model_name: props.row.model_name,
      correct_input_tokens: parsed.input_tokens,
      correct_output_tokens: parsed.output_tokens,
      correct_cache_read_tokens: parsed.cache_read_tokens,
      correct_cache_write_5m_tokens: parsed.cache_write_5m_tokens,
      correct_cache_write_1h_tokens: parsed.cache_write_1h_tokens,
      correct_cost_usd: parsed.cost_usd,
      reason: reason.trim(),
      ticket: ticket.trim(),
    })
  }

  const fields: {
    key: keyof CorrectedValues
    label: string
    step: string
  }[] = [
    { key: 'input_tokens', label: t('Input tokens'), step: '1' },
    { key: 'output_tokens', label: t('Output tokens'), step: '1' },
    { key: 'cache_read_tokens', label: t('Cache read tokens'), step: '1' },
    {
      key: 'cache_write_5m_tokens',
      label: t('Cache write 5m tokens'),
      step: '1',
    },
    {
      key: 'cache_write_1h_tokens',
      label: t('Cache write 1h tokens'),
      step: '1',
    },
    { key: 'cost_usd', label: t('Consumption (USD)'), step: '0.000001' },
  ]

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Correct hourly usage')}
      description={t(
        'The original hourly data remains unchanged. Reports and exports apply the saved difference.'
      )}
      contentClassName='sm:max-w-3xl'
      contentHeight='min(70vh, 720px)'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Close')}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!snapshotQuery.data || createMutation.isPending}
          >
            {t('Save correction')}
          </Button>
        </>
      }
    >
      <div className='space-y-5'>
        <div className='grid gap-3 rounded-lg border p-3 sm:grid-cols-4'>
          <div>
            <div className='text-muted-foreground text-xs'>{t('Date')}</div>
            <div className='font-medium'>{props.row?.date ?? '-'}</div>
          </div>
          <div>
            <div className='text-muted-foreground text-xs'>{t('Key ID')}</div>
            <div className='font-medium'>{props.row?.token_id ?? '-'}</div>
          </div>
          <div>
            <div className='text-muted-foreground text-xs'>
              {t('Model name')}
            </div>
            <div className='font-medium'>{props.row?.model_name ?? '-'}</div>
          </div>
          <div className='grid gap-1'>
            <Label htmlFor='usage-adjustment-hour'>{t('Hour (UTC+8)')}</Label>
            <NativeSelect
              id='usage-adjustment-hour'
              className='w-full'
              value={hour}
              onChange={(event) => setHour(Number(event.target.value))}
            >
              {Array.from({ length: 24 }, (_, value) => (
                <NativeSelectOption key={value} value={value}>
                  {String(value).padStart(2, '0')}:00–
                  {String(value).padStart(2, '0')}:59
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </div>
        </div>

        {snapshotQuery.isLoading && (
          <p className='text-muted-foreground text-sm'>{t('Loading...')}</p>
        )}
        {snapshotQuery.isError && (
          <p className='text-destructive text-sm'>
            {t(errorMessage(snapshotQuery.error))}
          </p>
        )}

        {snapshotQuery.data && (
          <>
            <div>
              <h3 className='mb-2 font-medium'>{t('Corrected values')}</h3>
              <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-3'>
                {fields.map((field) => (
                  <div key={field.key} className='grid gap-1.5'>
                    <Label htmlFor={`usage-adjustment-${field.key}`}>
                      {field.label}
                    </Label>
                    <Input
                      id={`usage-adjustment-${field.key}`}
                      type='number'
                      min='0'
                      step={field.step}
                      value={values[field.key]}
                      onChange={(event) =>
                        updateValue(field.key, event.target.value)
                      }
                    />
                  </div>
                ))}
              </div>
              <p className='text-muted-foreground mt-2 text-xs'>
                {t('Original')}:{' '}
                {formatCount(snapshotQuery.data.original.input_tokens)} /{' '}
                {formatCount(snapshotQuery.data.original.output_tokens)} tokens
                · {formatUsd(snapshotQuery.data.original.cost_usd)}
              </p>
            </div>

            <div className='grid gap-3 sm:grid-cols-2'>
              <div className='grid gap-1.5'>
                <Label htmlFor='usage-adjustment-ticket'>
                  {t('Related ticket')}
                </Label>
                <Input
                  id='usage-adjustment-ticket'
                  value={ticket}
                  maxLength={200}
                  onChange={(event) => setTicket(event.target.value)}
                />
              </div>
              <div className='grid gap-1.5 sm:col-span-2'>
                <Label htmlFor='usage-adjustment-reason'>
                  {t('Correction reason')} *
                </Label>
                <Textarea
                  id='usage-adjustment-reason'
                  value={reason}
                  maxLength={2000}
                  onChange={(event) => setReason(event.target.value)}
                />
              </div>
            </div>

            <div className='space-y-2'>
              <h3 className='font-medium'>{t('Adjustment history')}</h3>
              {snapshotQuery.data.adjustments.length === 0 ? (
                <p className='text-muted-foreground text-sm'>
                  {t('No adjustments for this hour')}
                </p>
              ) : (
                <div className='space-y-2'>
                  {snapshotQuery.data.adjustments.map((adjustment) => (
                    <div
                      key={adjustment.id}
                      className='rounded-lg border p-3 text-sm'
                    >
                      <div className='flex flex-wrap items-center justify-between gap-2'>
                        <div className='flex items-center gap-2'>
                          <span className='font-medium'>#{adjustment.id}</span>
                          <Badge
                            variant={
                              adjustment.reverted_at ? 'secondary' : 'outline'
                            }
                          >
                            {adjustment.reverted_at
                              ? t('Reverted')
                              : t('Active')}
                          </Badge>
                        </div>
                        {!adjustment.reverted_at && (
                          <Button
                            size='sm'
                            variant='outline'
                            disabled={
                              !revertReason.trim() || revertMutation.isPending
                            }
                            onClick={() => revertMutation.mutate(adjustment.id)}
                          >
                            {t('Revert')}
                          </Button>
                        )}
                      </div>
                      <p className='mt-2'>{adjustment.reason}</p>
                      <p className='text-muted-foreground mt-1 text-xs'>
                        {adjustment.operator_name} ·{' '}
                        {new Date(
                          adjustment.created_at * 1000
                        ).toLocaleString()}
                        {adjustment.ticket ? ` · ${adjustment.ticket}` : ''}
                      </p>
                      {adjustment.reverted_at > 0 && (
                        <p className='text-muted-foreground mt-1 text-xs'>
                          {t('Revert reason')}: {adjustment.revert_reason}
                        </p>
                      )}
                    </div>
                  ))}
                  <div className='grid gap-1.5'>
                    <Label htmlFor='usage-adjustment-revert-reason'>
                      {t('Revert reason')}
                    </Label>
                    <Input
                      id='usage-adjustment-revert-reason'
                      value={revertReason}
                      maxLength={2000}
                      onChange={(event) => setRevertReason(event.target.value)}
                    />
                  </div>
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </Dialog>
  )
}

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
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
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
import { useDebounce } from '@/hooks/use-debounce'

import {
  createSettlementConfig,
  searchUserOptions,
  updateSettlementConfig,
} from '../api'
import { DISCOUNT_MAX, DISCOUNT_MIN } from '../constants'
import type { SettlementConfig } from '../types'

const FORM_ID = 'settlement-config-form'

type FormValues = {
  user_id: string
  model_name: string
  discount: string
  input_price: string
  output_price: string
  request_price: string
}

const EMPTY_VALUES: FormValues = {
  user_id: '',
  model_name: '',
  discount: '1',
  input_price: '0',
  output_price: '0',
  request_price: '0',
}

function toFormValues(config: Partial<SettlementConfig>): FormValues {
  return {
    user_id: config.user_id === undefined ? '' : String(config.user_id),
    model_name: config.model_name ?? '',
    discount: String(config.discount ?? 1),
    input_price: String(config.input_price ?? 0),
    output_price: String(config.output_price ?? 0),
    request_price: String(config.request_price ?? 0),
  }
}

type SettlementConfigMutateDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Row being edited; when absent the dialog creates a new config */
  currentRow?: SettlementConfig
  /**
   * Row to copy values from while still creating a new config, or a bare
   * `{model_name}` / `{user_id}` seed for "add for this model / user".
   */
  seed?: Partial<SettlementConfig>
  onSaved: () => void
}

/**
 * Create/edit one settlement override. Shared by the Settlement Prices page
 * and the Pricing Center's customer-discount tab, which write the same rows.
 */
export function SettlementConfigMutateDialog({
  open,
  onOpenChange,
  currentRow,
  seed,
  onSaved,
}: SettlementConfigMutateDialogProps) {
  const { t } = useTranslation()
  const isEdit = currentRow !== undefined
  const [userKeyword, setUserKeyword] = useState('')
  const debouncedKeyword = useDebounce(userKeyword, 300)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const userOptionsQuery = useQuery({
    queryKey: ['settlement-config', 'user-options', debouncedKeyword],
    queryFn: () => searchUserOptions(debouncedKeyword),
    enabled: open && !isEdit,
  })

  const form = useForm<FormValues>({ defaultValues: EMPTY_VALUES })

  useEffect(() => {
    if (!open) return
    const base = currentRow ?? seed
    form.reset(base ? toFormValues(base) : EMPTY_VALUES)
  }, [currentRow, form, open, seed])

  const handleSubmit = async (values: FormValues) => {
    const parsed = Number.parseFloat(values.discount)
    const discount = Number.isNaN(parsed) || !parsed ? 1 : parsed
    if (discount < DISCOUNT_MIN || discount > DISCOUNT_MAX) {
      form.setError('discount', {
        message: t('Discount must be between 0.01 and 10.00'),
      })
      return
    }
    const payload = {
      user_id: Number.parseInt(values.user_id, 10),
      model_name: values.model_name.trim(),
      discount,
      input_price: Number.parseFloat(values.input_price) || 0,
      output_price: Number.parseFloat(values.output_price) || 0,
      request_price: Number.parseFloat(values.request_price) || 0,
    }
    setIsSubmitting(true)
    try {
      const res = currentRow
        ? await updateSettlementConfig({ ...payload, id: currentRow.id })
        : await createSettlementConfig(payload)
      if (!res.success) {
        toast.error(res.message || t('Operation failed'))
        return
      }
      toast.success(
        isEdit ? t('Updated successfully') : t('Created successfully')
      )
      onOpenChange(false)
      onSaved()
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        isEdit ? t('Edit settlement price') : t('Add settlement price')
      }
      contentClassName='sm:max-w-md'
      contentHeight='auto'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' form={FORM_ID} disabled={isSubmitting}>
            {t('Save')}
          </Button>
        </>
      }
    >
      <form
        id={FORM_ID}
        onSubmit={form.handleSubmit(handleSubmit)}
        className='space-y-4'
      >
        <div className='grid gap-1.5'>
          <Label htmlFor='settlement-user'>{t('User')} *</Label>
          {isEdit ? (
            <Input
              id='settlement-user'
              disabled
              value={`${currentRow?.username ?? ''} (ID: ${currentRow?.user_id})`}
            />
          ) : (
            <Select
              items={userOptionsQuery.data?.map((option) => ({
                value: String(option.value),
                label: option.label,
              }))}
              value={form.watch('user_id')}
              onValueChange={(value) =>
                form.setValue('user_id', value ?? '', { shouldValidate: true })
              }
            >
              <SelectTrigger id='settlement-user'>
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
          )}
        </div>

        <div className='grid gap-1.5'>
          <Label htmlFor='settlement-model'>{t('Model name')} *</Label>
          <Input
            id='settlement-model'
            placeholder={t('Model name')}
            {...form.register('model_name', { required: true })}
          />
        </div>

        <div className='grid gap-1.5'>
          <Label htmlFor='settlement-discount'>{t('Model discount')}</Label>
          <Input
            id='settlement-discount'
            type='number'
            min={DISCOUNT_MIN}
            max={DISCOUNT_MAX}
            step={0.01}
            placeholder='1'
            {...form.register('discount')}
          />
          <p className='text-muted-foreground text-xs'>
            {t(
              'For example 0.8 bills at 80%; range 0.01 – 10.00, where 1 means no discount'
            )}
          </p>
          {form.formState.errors.discount?.message && (
            <p className='text-destructive text-xs'>
              {form.formState.errors.discount.message}
            </p>
          )}
        </div>

        <div className='grid gap-1.5'>
          <Label htmlFor='settlement-input-price'>
            {t('Input price')} ($/1M tokens)
          </Label>
          <Input
            id='settlement-input-price'
            type='number'
            min={0}
            step={0.0001}
            {...form.register('input_price')}
          />
        </div>

        <div className='grid gap-1.5'>
          <Label htmlFor='settlement-output-price'>
            {t('Output price')} ($/1M tokens)
          </Label>
          <Input
            id='settlement-output-price'
            type='number'
            min={0}
            step={0.0001}
            {...form.register('output_price')}
          />
        </div>

        <div className='grid gap-1.5'>
          <Label htmlFor='settlement-request-price'>
            {t('Per-call price')} ($)
          </Label>
          <Input
            id='settlement-request-price'
            type='number'
            min={0}
            step={0.0001}
            {...form.register('request_price')}
          />
        </div>
      </form>
    </Dialog>
  )
}

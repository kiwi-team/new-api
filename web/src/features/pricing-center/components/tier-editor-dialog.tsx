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
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { DEFAULT_FIRST_TIER_MAX_TOKENS } from '../constants'
import type { PriceTier } from '../types'

const TIER_FORM_ID = 'pricing-center-tier-form'

type TierFormValues = {
  max_tokens: number
  input_price: number
  output_price: number
  cached_input_price: string
  cache_write_price: string
}

type TierEditorDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  modelName: string
  /** Tier being edited, or undefined when adding a new one */
  tier?: PriceTier
  /** Ceilings already taken by other tiers of this model */
  usedMaxTokens: number[]
  onSave: (tier: PriceTier) => void
}

export function TierEditorDialog({
  open,
  onOpenChange,
  modelName,
  tier,
  usedMaxTokens,
  onSave,
}: TierEditorDialogProps) {
  const { t } = useTranslation()
  const isEdit = tier !== undefined

  const form = useForm<TierFormValues>({
    defaultValues: {
      max_tokens: DEFAULT_FIRST_TIER_MAX_TOKENS,
      input_price: 0,
      output_price: 0,
      cached_input_price: '',
      cache_write_price: '',
    },
  })

  useEffect(() => {
    if (!open) return
    form.reset({
      max_tokens: tier?.max_tokens ?? DEFAULT_FIRST_TIER_MAX_TOKENS,
      input_price: tier?.input_price ?? 0,
      output_price: tier?.output_price ?? 0,
      cached_input_price:
        tier?.cached_input_price === undefined
          ? ''
          : String(tier.cached_input_price),
      cache_write_price:
        tier?.cache_write_price === undefined
          ? ''
          : String(tier.cache_write_price),
    })
  }, [form, open, tier])

  const handleSave = (values: TierFormValues) => {
    const maxTokens = Number(values.max_tokens)
    if (!isEdit && usedMaxTokens.includes(maxTokens)) {
      form.setError('max_tokens', {
        message: t('This threshold already exists'),
      })
      return
    }
    const next: PriceTier = {
      max_tokens: maxTokens,
      input_price: Number(values.input_price),
      output_price: Number(values.output_price),
    }
    // Cache prices are optional: an empty field means "fall back to the ratio",
    // so it must stay absent rather than become 0.
    if (values.cached_input_price !== '') {
      next.cached_input_price = Number(values.cached_input_price)
    }
    if (values.cache_write_price !== '') {
      next.cache_write_price = Number(values.cache_write_price)
    }
    onSave(next)
    onOpenChange(false)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={isEdit ? t('Edit price tier') : t('Add price tier')}
      description={`${t('Model')}: ${modelName}`}
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
          <Button type='submit' form={TIER_FORM_ID}>
            {t('Save')}
          </Button>
        </>
      }
    >
      <form
        id={TIER_FORM_ID}
        onSubmit={form.handleSubmit(handleSave)}
        className='space-y-4'
      >
        <div className='grid gap-1.5'>
          <Label htmlFor='tier-max-tokens'>{t('Input token limit')} *</Label>
          <Input
            id='tier-max-tokens'
            type='number'
            min={1}
            placeholder='128000'
            {...form.register('max_tokens', { required: true, min: 1 })}
          />
          {form.formState.errors.max_tokens?.message && (
            <p className='text-destructive text-xs'>
              {form.formState.errors.max_tokens.message}
            </p>
          )}
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='tier-input-price'>
            {t('Input price ($/1M tokens)')} *
          </Label>
          <Input
            id='tier-input-price'
            type='number'
            min={0}
            step={0.01}
            placeholder='0.5'
            {...form.register('input_price', { required: true, min: 0 })}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='tier-output-price'>
            {t('Output price ($/1M tokens)')} *
          </Label>
          <Input
            id='tier-output-price'
            type='number'
            min={0}
            step={0.01}
            placeholder='2.0'
            {...form.register('output_price', { required: true, min: 0 })}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='tier-cached-input-price'>
            {t('Cache read price ($/1M tokens)')}
          </Label>
          <Input
            id='tier-cached-input-price'
            type='number'
            min={0}
            step={0.01}
            placeholder={t('Leave empty to fall back to the cache ratio')}
            {...form.register('cached_input_price')}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor='tier-cache-write-price'>
            {t('Cache write price ($/1M tokens)')}
          </Label>
          <Input
            id='tier-cache-write-price'
            type='number'
            min={0}
            step={0.01}
            placeholder={t('Leave empty to fall back to the cache write ratio')}
            {...form.register('cache_write_price')}
          />
        </div>
      </form>
    </Dialog>
  )
}

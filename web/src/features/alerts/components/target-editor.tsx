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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'

import { saveTarget } from '../api'
import type { AlertTarget, Catalog } from '../types'

const schema = z.object({
  name: z.string().trim().min(1).max(160),
  type: z.string(),
  address: z.string().max(4096),
  secret: z.string().max(4096),
  token: z.string().max(4096),
  enabled: z.boolean(),
  language: z.string(),
  replace: z.boolean(),
  platform: z.boolean(),
  privateNetwork: z.boolean(),
})
type Values = z.infer<typeof schema>
type Props = {
  target?: AlertTarget
  catalog: Catalog
  onClose: () => void
  onSaved: (owner?: number) => void
}

export function TargetEditor(props: Props) {
  const { t } = useTranslation()
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: props.target?.name ?? '',
      type: props.target?.type ?? 'email',
      address: '',
      secret: '',
      token: '',
      enabled: props.target?.enabled ?? true,
      language: props.target?.language ?? 'zh-CN',
      replace: !props.target,
      platform: props.target?.owner_user_id === 0,
      privateNetwork: false,
    },
  })
  const values = form.watch()
  const mutation = useMutation({
    mutationFn: async (input: Values) => {
      if (input.replace && !input.address) {
        form.setError('address', { message: t('Required') })
        return false
      }
      await saveTarget(
        {
          name: input.name,
          type: input.type,
          enabled: input.enabled,
          language: input.language,
          expected_revision: props.target?.revision ?? 0,
          credentials: input.replace
            ? {
                address: input.address,
                secret: input.secret,
                token: input.token,
                priority: 5,
                private_network: input.privateNetwork,
              }
            : undefined,
        },
        props.target?.id,
        input.platform ? 0 : props.target?.owner_user_id
      )
      return true
    },
    onSuccess: (saved, input) => {
      if (saved) props.onSaved(input.platform ? 0 : props.target?.owner_user_id)
    },
  })
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) props.onClose()
      }}
      title={t(
        props.target ? 'Edit notification target' : 'Create notification target'
      )}
      description={t(
        'Saved credentials are encrypted and are never returned to the browser.'
      )}
      footer={
        <>
          <Button
            variant='outline'
            disabled={mutation.isPending}
            onClick={props.onClose}
          >
            {t('Cancel')}
          </Button>
          <Button
            form='alert-target-form'
            type='submit'
            disabled={mutation.isPending}
          >
            {t('Save')}
          </Button>
        </>
      }
    >
      <form
        id='alert-target-form'
        className='space-y-4'
        onSubmit={form.handleSubmit((values) => mutation.mutate(values))}
      >
        <div className='space-y-1'>
          <Label htmlFor='target-name'>{t('Name')}</Label>
          <Input
            id='target-name'
            {...form.register('name')}
            aria-invalid={!!form.formState.errors.name}
          />
        </div>
        <div className='space-y-1'>
          <Label htmlFor='target-type'>{t('Type')}</Label>
          <NativeSelect
            className='w-full'
            id='target-type'
            disabled={!!props.target}
            {...form.register('type')}
          >
            {props.catalog.target_types.map((type) => (
              <NativeSelectOption key={type} value={type}>
                {type}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </div>
        {props.catalog.manage_platform && !props.target && (
          <div className='flex gap-2'>
            <Switch
              id='target-platform'
              checked={values.platform}
              onCheckedChange={(value) => form.setValue('platform', value)}
            />
            <Label htmlFor='target-platform'>
              {t('Platform notification target')}
            </Label>
          </div>
        )}
        {!!props.target && (
          <>
            <p className='text-muted-foreground text-sm break-all'>
              {props.target.address_hint}
            </p>
            <div className='flex gap-2'>
              <Switch
                id='target-replace'
                checked={values.replace}
                onCheckedChange={(value) => form.setValue('replace', value)}
              />
              <Label htmlFor='target-replace'>{t('Replace credentials')}</Label>
            </div>
          </>
        )}
        {values.replace && (
          <>
            <div className='space-y-1'>
              <Label htmlFor='target-address'>
                {t(values.type === 'email' ? 'Email' : 'HTTPS destination URL')}
              </Label>
              <Input
                id='target-address'
                type={values.type === 'email' ? 'email' : 'password'}
                autoComplete='off'
                {...form.register('address')}
                aria-invalid={!!form.formState.errors.address}
              />
            </div>
            {['feishu', 'webhook_v1', 'webhook_v2'].includes(values.type) && (
              <div className='space-y-1'>
                <Label htmlFor='target-secret'>{t('Signing secret')}</Label>
                <Input
                  id='target-secret'
                  type='password'
                  autoComplete='new-password'
                  {...form.register('secret')}
                />
              </div>
            )}
            {values.type === 'gotify' && (
              <div className='space-y-1'>
                <Label htmlFor='target-token'>{t('Application token')}</Label>
                <Input
                  id='target-token'
                  type='password'
                  autoComplete='new-password'
                  {...form.register('token')}
                />
              </div>
            )}
          </>
        )}
        {props.catalog.manage_platform &&
          values.platform &&
          values.replace &&
          values.type !== 'email' && (
            <div className='flex gap-2'>
              <Switch
                id='target-private'
                checked={values.privateNetwork}
                onCheckedChange={(value) =>
                  form.setValue('privateNetwork', value)
                }
              />
              <Label htmlFor='target-private'>
                {t('Allow approved private network destinations')}
              </Label>
            </div>
          )}
        <div className='space-y-1'>
          <Label htmlFor='target-language'>{t('Language')}</Label>
          <NativeSelect
            id='target-language'
            className='w-full'
            {...form.register('language')}
          >
            <NativeSelectOption value='zh-CN'>简体中文</NativeSelectOption>
            <NativeSelectOption value='zh-TW'>繁體中文</NativeSelectOption>
            <NativeSelectOption value='en'>English</NativeSelectOption>
          </NativeSelect>
        </div>
        <div className='flex gap-2'>
          <Switch
            id='target-enabled'
            checked={values.enabled}
            onCheckedChange={(value) => form.setValue('enabled', value)}
          />
          <Label htmlFor='target-enabled'>{t('Enabled')}</Label>
        </div>
        {Object.keys(form.formState.errors).length > 0 && (
          <p role='alert' className='text-destructive text-sm'>
            {t('Check the destination and required fields.')}
          </p>
        )}
      </form>
    </Dialog>
  )
}

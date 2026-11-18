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
import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  SettingsControlChildren,
  SettingsControlGroup,
  SettingsSwitchField,
} from '@/features/system-settings/components/settings-form-layout'

import type { ChannelFormValues } from '../lib/channel-form'

export function GoogleRequestPrivacySetting(props: {
  channelType: number
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  const policy = useWatch({
    control: form.control,
    name: 'google_request_privacy',
  })
  const pluginKey = useWatch({ control: form.control, name: 'task_plugin_key' })
  const googlePlugin =
    props.channelType === 61 &&
    (pluginKey === 'google' || pluginKey === 'vertex-ai')
  if (props.channelType !== 24 && props.channelType !== 41 && !googlePlugin) {
    return null
  }

  return (
    <SettingsControlGroup>
      <FormField
        control={form.control}
        name='google_request_privacy'
        render={({ field }) => (
          <FormItem>
            <SettingsSwitchField
              controlId='google-request-privacy'
              label={t('Google request privacy')}
              description={t(
                'Only applies to direct Google requests. Removes client IP, origin and tracing headers. Disabled by default.'
              )}
              checked={field.value?.enabled === true}
              onCheckedChange={(enabled) =>
                field.onChange({
                  ...form.getValues('google_request_privacy'),
                  enabled,
                })
              }
              disabled={props.disabled}
            />
            <FormMessage />
          </FormItem>
        )}
      />
      {policy?.enabled === true && (
        <SettingsControlChildren className='space-y-3'>
          <FormField
            control={form.control}
            name='google_request_privacy.user_agent_mode'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('User agent handling')}</FormLabel>
                <FormControl>
                  <NativeSelect
                    {...field}
                    value={field.value ?? 'preserve'}
                    disabled={props.disabled}
                  >
                    <NativeSelectOption value='preserve'>
                      {t('Keep existing')}
                    </NativeSelectOption>
                    <NativeSelectOption value='custom'>
                      {t('Use custom user agent')}
                    </NativeSelectOption>
                    <NativeSelectOption value='omit'>
                      {t('Do not send')}
                    </NativeSelectOption>
                  </NativeSelect>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          {policy.user_agent_mode === 'custom' && (
            <FormField
              control={form.control}
              name='google_request_privacy.custom_user_agent'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Custom user agent')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value ?? ''}
                      disabled={props.disabled}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}
          <FormField
            control={form.control}
            name='google_request_privacy.sdk_version_headers'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('SDK version headers')}</FormLabel>
                <FormControl>
                  <NativeSelect
                    {...field}
                    value={field.value ?? 'preserve'}
                    disabled={props.disabled}
                  >
                    <NativeSelectOption value='preserve'>
                      {t('Keep existing')}
                    </NativeSelectOption>
                    <NativeSelectOption value='omit'>
                      {t('Do not send')}
                    </NativeSelectOption>
                  </NativeSelect>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsControlChildren>
      )}
    </SettingsControlGroup>
  )
}

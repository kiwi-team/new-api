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

import { MultiSelect } from '@/components/multi-select'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'

import type { ChannelFormValues } from '../lib/channel-form'
import { parseModelsString } from '../lib/model-mapping-validation'

export function BufferedStreamSetting(props: { disabled?: boolean }) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  const models = useWatch({ control: form.control, name: 'models' })
  const selected =
    useWatch({
      control: form.control,
      name: 'force_upstream_stream_for_non_stream_models',
    }) ?? []
  const available = parseModelsString(models ?? '')
  const known = new Set(available)
  const orphaned = selected.filter((model) => !known.has(model))
  const options = [
    ...available.map((model) => ({ value: model, label: model })),
    ...orphaned.map((model) => ({
      value: model,
      label: t('{{model}} (unavailable)', { model }),
      hint: t(
        'Model removed from this channel; remove it here to clear the rule.'
      ),
    })),
  ]

  return (
    <FormField
      control={form.control}
      name='force_upstream_stream_for_non_stream_models'
      render={({ field }) => (
        <FormItem>
          <FormLabel>
            {t('Use upstream streaming for non-streaming requests')}
          </FormLabel>
          <FormControl>
            <MultiSelect
              placeholder={t(
                'Use upstream streaming for non-streaming requests'
              )}
              options={options}
              selected={field.value ?? []}
              onChange={field.onChange}
              disabled={props.disabled}
              allowCreate={false}
              maxVisibleChips={8}
            />
          </FormControl>
          <FormDescription>
            {t(
              'For Chat Completions and Messages, stream from the upstream and return complete JSON to the client. Overrides stream settings, including body passthrough. Client streaming is unchanged.'
            )}
            {orphaned.length > 0 && (
              <span className='block'>
                {t(
                  'Model removed from this channel; remove it here to clear the rule.'
                )}
              </span>
            )}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

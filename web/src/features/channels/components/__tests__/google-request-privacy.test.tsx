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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
import { describe, expect, test } from 'vitest'

import { Form } from '@/components/ui/form'

import {
  buildSettingJSON,
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  type ChannelFormValues,
} from '../../lib/channel-form'
import { channelSchema } from '../../types'
import { GoogleRequestPrivacySetting } from '../google-request-privacy-setting'

function PrivacyForm(props: {
  channelType: number
  disabled?: boolean
  onSave: (value: string) => void
}) {
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: props.channelType,
      task_plugin_key: 'google',
    },
  })
  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit((values) =>
          props.onSave(buildSettingJSON(values))
        )}
      >
        <GoogleRequestPrivacySetting
          channelType={props.channelType}
          disabled={props.disabled}
        />
        <button type='submit'>Save</button>
      </form>
    </Form>
  )
}

describe('Google request privacy settings', () => {
  test.each([24, 41, 61])(
    'channel type %s preserves privacy and existing settings when edited',
    (type) => {
      const privacy = {
        enabled: true,
        user_agent_mode: 'custom',
        custom_user_agent: 'example-client/1.0',
        sdk_version_headers: 'omit',
      }
      const channel = channelSchema.parse({
        id: 1,
        name: 'Google',
        type,
        key: 'test-key',
        status: 1,
        created_time: 0,
        test_time: 0,
        response_time: 0,
        used_quota: 0,
        balance_updated_time: 0,
        models: 'gemini-test',
        group: 'default',
        setting: JSON.stringify({
          google_request_privacy: privacy,
          proxy: 'http://localhost:8080',
          google_file_bucket: 'files',
        }),
      })
      const form = transformChannelToFormDefaults(channel)
      const saved = JSON.parse(
        buildSettingJSON({ ...form, name: 'Edited channel' })
      )
      expect(saved).toMatchObject({
        google_request_privacy: privacy,
        proxy: 'http://localhost:8080',
        google_file_bucket: 'files',
      })
    }
  )

  test('new channels omit the policy and existing behavior remains the default', () => {
    expect(
      JSON.parse(buildSettingJSON(CHANNEL_FORM_DEFAULT_VALUES))
    ).not.toHaveProperty('google_request_privacy')
  })

  test('an unfinished custom user agent can be saved while disabled', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Google',
      type: 24,
      key: 'test-key',
      models: 'gemini-test',
      google_request_privacy: {
        enabled: false,
        user_agent_mode: 'custom',
        custom_user_agent: '',
      },
    })
    expect(result.success).toBe(true)
  })

  test('missing enabled and empty mode values keep their backend defaults when edited', () => {
    const channel = channelSchema.parse({
      id: 1,
      name: 'Google',
      key: 'test-key',
      type: 24,
      status: 1,
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      models: 'gemini-test',
      setting: JSON.stringify({
        google_request_privacy: {
          user_agent_mode: '',
          sdk_version_headers: '',
        },
      }),
    })
    const defaults = transformChannelToFormDefaults(channel)
    expect(defaults.google_request_privacy?.enabled).toBe(false)
    expect(defaults.google_request_privacy?.user_agent_mode).toBeUndefined()
    expect(defaults.google_request_privacy?.sdk_version_headers).toBeUndefined()
    expect(channelFormSchema.safeParse(defaults).success).toBe(true)
  })

  test.each([24, 41, 61])(
    'channel type %s can enable, configure and disable privacy without losing choices',
    async (channelType) => {
      const saved: string[] = []
      const user = userEvent.setup()
      render(
        <PrivacyForm
          channelType={channelType}
          onSave={(value) => saved.push(value)}
        />
      )
      const toggle = screen.getByRole('switch', {
        name: 'Google request privacy',
      })
      expect(toggle).not.toBeChecked()
      expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
      await user.click(toggle)
      await user.selectOptions(
        screen.getByRole('combobox', { name: 'User agent handling' }),
        'custom'
      )
      await user.type(
        screen.getByRole('textbox', { name: 'Custom user agent' }),
        'example-client/1.0'
      )
      await user.selectOptions(
        screen.getByRole('combobox', { name: 'SDK version headers' }),
        'omit'
      )
      await user.click(screen.getByRole('button', { name: 'Save' }))
      expect(JSON.parse(saved[0]).google_request_privacy).toEqual({
        enabled: true,
        user_agent_mode: 'custom',
        custom_user_agent: 'example-client/1.0',
        sdk_version_headers: 'omit',
      })
      toggle.focus()
      await user.keyboard(' ')
      await user.click(screen.getByRole('button', { name: 'Save' }))
      expect(JSON.parse(saved[1]).google_request_privacy).toEqual({
        enabled: false,
        user_agent_mode: 'custom',
        custom_user_agent: 'example-client/1.0',
        sdk_version_headers: 'omit',
      })
    }
  )

  test('locked channels cannot enable privacy', async () => {
    const user = userEvent.setup()
    render(<PrivacyForm channelType={24} disabled onSave={() => {}} />)
    const toggle = screen.getByRole('switch', {
      name: 'Google request privacy',
    })
    expect(toggle).toHaveAttribute('aria-disabled', 'true')
    await user.click(toggle)
    expect(toggle).not.toBeChecked()
  })

  test('unrelated channels do not expose the Google control', () => {
    render(<PrivacyForm channelType={1} onSave={() => {}} />)
    expect(
      screen.queryByRole('switch', { name: 'Google request privacy' })
    ).not.toBeInTheDocument()
  })

  test.each(['', 'line\r\ninjection', '中'.repeat(86)])(
    'rejects invalid custom user agent %s',
    (customUserAgent) => {
      const parsed = channelFormSchema.safeParse({
        ...CHANNEL_FORM_DEFAULT_VALUES,
        name: 'Google',
        type: 24,
        key: 'test-key',
        models: 'gemini-test',
        google_request_privacy: {
          enabled: true,
          user_agent_mode: 'custom',
          custom_user_agent: customUserAgent,
        },
      })
      expect(parsed.success).toBe(false)
      if (!parsed.success) {
        expect(parsed.error.issues).toContainEqual(
          expect.objectContaining({
            path: ['google_request_privacy', 'custom_user_agent'],
          })
        )
      }
    }
  )
})

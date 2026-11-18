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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
import { describe, expect, test, vi } from 'vitest'

import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'

import {
  buildSettingJSON,
  CHANNEL_FORM_DEFAULT_VALUES,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  type ChannelFormValues,
} from '../../lib/channel-form'
import { channelSchema } from '../../types'
import { BufferedStreamSetting } from '../buffered-stream-setting'

function BridgeForm(props: {
  models?: string
  selected?: string[]
  disabled?: boolean
  onSave: (setting: string) => void
}) {
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      models: props.models ?? 'm1,m2',
      force_upstream_stream_for_non_stream_models: props.selected ?? [],
    },
  })
  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit((values) =>
          props.onSave(buildSettingJSON(values))
        )}
      >
        <BufferedStreamSetting disabled={props.disabled} />
        <Button type='submit'>Save</Button>
      </form>
    </Form>
  )
}

describe('buffered upstream streaming policy', () => {
  test('existing channels round trip selected and orphaned models inside setting JSON', () => {
    const channel = channelSchema.parse({
      id: 1,
      name: 'Bridge',
      type: 1,
      key: 'test-key',
      status: 1,
      created_time: 0,
      test_time: 0,
      response_time: 0,
      used_quota: 0,
      balance_updated_time: 0,
      models: 'm1,m2',
      group: 'default',
      setting: JSON.stringify({
        proxy: 'http://localhost:8080',
        model_behaviors: {
          m1: { force_upstream_stream_for_non_stream: true },
          orphan: { force_upstream_stream_for_non_stream: true },
        },
      }),
    })
    const values = transformChannelToFormDefaults(channel)
    expect(values.force_upstream_stream_for_non_stream_models).toEqual([
      'm1',
      'orphan',
    ])
    const payload = transformFormDataToCreatePayload(values)
    expect(payload.channel).not.toHaveProperty(
      'force_upstream_stream_for_non_stream_models'
    )
    expect(JSON.parse(payload.channel.setting ?? '{}')).toMatchObject({
      proxy: 'http://localhost:8080',
      model_behaviors: {
        m1: { force_upstream_stream_for_non_stream: true },
        orphan: { force_upstream_stream_for_non_stream: true },
      },
    })
    expect(
      JSON.parse(
        buildSettingJSON({
          ...values,
          force_upstream_stream_for_non_stream_models: [],
        })
      )
    ).not.toHaveProperty('model_behaviors')
    expect(
      JSON.parse(buildSettingJSON(CHANNEL_FORM_DEFAULT_VALUES))
    ).not.toHaveProperty('model_behaviors')
  })
  test('models can be searched and selected with the keyboard', async () => {
    const user = userEvent.setup()
    const save = vi.fn()
    render(<BridgeForm onSave={save} />)
    const input = screen.getByRole('combobox', {
      name: 'Use upstream streaming for non-streaming requests',
    })
    await user.click(input)
    await user.type(input, 'm2')
    await screen.findByRole('option', { name: 'm2' })
    await user.keyboard('{ArrowDown}')
    await user.keyboard('{Enter}')
    await waitFor(() => expect(screen.getByText('m2')).toBeInTheDocument())
    expect(save).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(save).toHaveBeenCalled())
    expect(JSON.parse(save.mock.calls[0][0]).model_behaviors).toEqual({
      m2: { force_upstream_stream_for_non_stream: true },
    })
  })
  test('removed models stay visible and can be removed without changing other rules', async () => {
    const user = userEvent.setup()
    const save = vi.fn()
    render(<BridgeForm selected={['m1', 'orphan']} onSave={save} />)
    expect(screen.getByText('orphan (unavailable)')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Model removed from this channel; remove it here to clear the rule.'
      )
    ).toBeInTheDocument()
    await user.click(screen.getByRole('combobox'))
    await user.click(
      screen.getByRole('option', { name: 'orphan (unavailable)' })
    )
    await user.keyboard('{Escape}')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(save).toHaveBeenCalled())
    expect(JSON.parse(save.mock.calls[0][0]).model_behaviors).toEqual({
      m1: { force_upstream_stream_for_non_stream: true },
    })
  })
  test('unknown models cannot be created in the policy selector', async () => {
    const user = userEvent.setup()
    const save = vi.fn()
    render(<BridgeForm onSave={save} />)
    const input = screen.getByRole('combobox')
    await user.click(input)
    await user.type(input, 'unknown-model')
    await user.keyboard('{Enter}{Escape}')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(save).toHaveBeenCalled())
    expect(JSON.parse(save.mock.calls[0][0])).not.toHaveProperty(
      'model_behaviors'
    )
  })
  test('sensitive settings lock disables the selector and preserves saved rules', async () => {
    const user = userEvent.setup()
    const save = vi.fn()
    render(<BridgeForm disabled selected={['m1']} onSave={save} />)
    expect(screen.getByRole('combobox')).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(save).toHaveBeenCalled())
    expect(JSON.parse(save.mock.calls[0][0]).model_behaviors).toEqual({
      m1: { force_upstream_stream_for_non_stream: true },
    })
  })
})

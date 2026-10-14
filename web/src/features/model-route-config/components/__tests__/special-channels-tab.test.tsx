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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SpecialChannelsTab } from '../special-channels-tab'

function renderTab() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <SpecialChannelsTab />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: [
        {
          key: 'VideoChannels',
          value: '[{"model_name":"old","channel_ids":[[1]]}]',
        },
      ],
    },
  })
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
})

test('legacy settings show no rules and saving uses only the new configuration keys', async () => {
  renderTab()
  const save = await screen.findByRole('button', { name: 'Save' })
  await waitFor(() => expect(save).toBeEnabled())
  expect(screen.getByText('No large file rules')).toBeVisible()
  expect(
    screen.getByText(
      'Only URL attachments use the file size threshold. Base64 stays inline and does not trigger large file routing.'
    )
  ).toBeVisible()
  expect(screen.getByLabelText('URL file threshold (bytes)')).toHaveValue(
    10485760
  )
  expect(screen.queryByDisplayValue('old')).not.toBeInTheDocument()
  expect(
    screen.getByRole('switch', { name: 'Large file routing' })
  ).not.toBeChecked()
  fireEvent.change(
    screen.getByLabelText('Chat model patterns (one per line)'),
    { target: { value: '^gemini-\n\n' } }
  )
  fireEvent.click(save)
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(2))
  expect(api.put).toHaveBeenNthCalledWith(1, '/api/option/', {
    key: 'LargeFileChannels',
    value: '[]',
  })
  expect(api.put).toHaveBeenNthCalledWith(2, '/api/option/', {
    key: 'GeminiFileRouting',
    value: JSON.stringify({
      enabled: false,
      mode: 'forward',
      chat_model_patterns: ['^gemini-'],
      inline_threshold_bytes: 10485760,
      max_inline_request_bytes: 33554432,
    }),
  })
})

test('saves an upstream request limit above 256 MiB without a validation error', async () => {
  renderTab()
  const save = await screen.findByRole('button', { name: 'Save' })
  await waitFor(() => expect(save).toBeEnabled())
  const input = screen.getByLabelText('Upstream request limit (bytes)')
  fireEvent.change(input, { target: { value: '335544320' } })
  expect(input).toBeValid()
  fireEvent.click(save)
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(2))
  const saved = vi.mocked(api.put).mock.calls[1][1] as {
    key: string
    value: string
  }
  expect(saved.key).toBe('GeminiFileRouting')
  expect(JSON.parse(saved.value).max_inline_request_bytes).toBe(335544320)
})

test('invalid saved rules show an error and cannot be overwritten by an empty form', async () => {
  vi.mocked(api.get).mockResolvedValue({
    data: {
      success: true,
      data: [{ key: 'LargeFileChannels', value: '{broken' }],
    },
  })
  renderTab()
  expect(
    await screen.findByText(
      'File routing configuration is invalid. Correct the saved configuration before editing.'
    )
  ).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument()
  expect(api.put).not.toHaveBeenCalled()
})

test('failed rule save does not enable the file processing policy', async () => {
  vi.mocked(api.put).mockResolvedValue({
    data: { success: false, message: 'save failed' },
  })
  renderTab()
  const save = await screen.findByRole('button', { name: 'Save' })
  await waitFor(() => expect(save).toBeEnabled())
  fireEvent.click(save)
  await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
  await waitFor(() => expect(save).toBeEnabled())
})

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
import { act, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { expect, test } from 'vitest'

import { RequestTimingViewer } from '@/components/request-timing-viewer'
import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import vi from '@/i18n/locales/vi.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zh from '@/i18n/locales/zh.json'

async function renderTiming(data: unknown, language = 'en') {
  const i18n = createInstance()
  await i18n.init({
    lng: language,
    fallbackLng: 'en',
    resources: { en, zh, zhCN: zh, zhTW, fr, ja, ru, vi },
    interpolation: { escapeValue: false },
  })
  render(
    <I18nextProvider i18n={i18n}>
      <RequestTimingViewer data={data} />
    </I18nextProvider>
  )
  return i18n
}

const snapshot = {
  schema_version: 1,
  phase: 'error_log',
  request: {
    gateway_total_ms: null,
    pre_upstream_ms: 397000,
    body_read_ms: 390000,
    route_ms: 0,
    offsets_ms: { gateway_entry_at_ms: 0 },
  },
  business_spans: [
    { phase: 'task_wait', duration_ms: null, status: 'pending_at_finalize' },
  ],
  attempts: [
    {
      attempt_id: 'attempt-1',
      channel_id: 1567,
      attempt_started_at_ms: 397000,
      attempt_ended_at_ms: 400000,
      first_sse_data_at_ms: 397003,
      first_output_at_ms: null,
    },
  ],
  outbound_calls: [
    {
      call_id: 'call-1',
      attempt_id: 'attempt-1',
      purpose: 'inference',
      call_total_ms: 3000,
      connection_wait_ms: 0,
      upstream_body_write_ms: 5,
      upstream_wait_headers_ms: 2950,
      upstream_receive_span_ms: 20,
      upstream_read_call_sum_ms: 20,
    },
  ],
  downstream: {
    write_span_ms: 10,
    write_call_sum_ms: 0.0005,
    write_call_max_ms: 0.0005,
    flush_call_sum_ms: null,
  },
  vendor_extension: { retained: true },
}

test('summary separates request stages, attempts, HTTP calls, and local writes without inventing a total', async () => {
  await renderTiming(snapshot)
  expect(screen.getByRole('tab', { name: 'Stage summary' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  expect(
    screen.getByText('Before the first upstream request').nextElementSibling
  ).toHaveTextContent('397 s')
  expect(
    screen.getByText('Request total').nextElementSibling
  ).toHaveTextContent('Pending at snapshot')
  expect(
    screen.getByText('Channel routing').nextElementSibling
  ).toHaveTextContent('0 ms')
  expect(screen.getByText('First output').nextElementSibling).toHaveTextContent(
    'Not recorded'
  )
  expect(
    screen.getByText('Sending the upstream request').nextElementSibling
  ).toHaveTextContent('5 ms')
  expect(
    screen.getByText('Waiting for upstream response headers').nextElementSibling
  ).toHaveTextContent('2.95 s')
  expect(
    screen.getByText('Downstream writes (sum)').nextElementSibling
  ).toHaveTextContent('< 0.01 ms')
  expect(
    screen.getByText(
      'Streaming stages overlap; do not add these durations. Header waiting includes network and upstream processing time.'
    )
  ).toBeVisible()
  expect(
    screen.getByText('Event times are offsets from gateway entry.')
  ).toBeVisible()
  expect(
    screen.getByText(
      'Downstream times measure local writes and flushes, not confirmation that the client received the data.'
    )
  ).toBeVisible()
})

test('new parsing metrics distinguish authentication decoding, business validation and its decode substep', async () => {
  await renderTiming({
    schema_version: 2,
    request: { request_parse_ms: 29 },
    business_spans: [
      { phase: 'auth_tag_decode', duration_ms: 44 },
      { phase: 'request_parse', duration_ms: 29 },
      { phase: 'request_decode', duration_ms: 29 },
    ],
  })
  const stages = within(screen.getByRole('region', { name: 'Request stages' }))
  expect(
    stages.getByText('Business request parsing and validation (overall)')
      .nextElementSibling
  ).toHaveTextContent('29 ms')
  const spans = within(
    screen.getByRole('region', { name: 'Processing intervals' })
  )
  expect(
    spans.getByText('Authentication tag decoding').nextElementSibling
  ).toHaveTextContent('44 ms')
  expect(
    spans.getByText('Business request parsing and validation (overall)')
      .nextElementSibling
  ).toHaveTextContent('29 ms')
  expect(
    spans.getByText('Request body decoding (substep)').nextElementSibling
  ).toHaveTextContent('29 ms')
  expect(
    spans.getByText(
      'Decoding substeps can be included in enclosing stages; do not add their durations.'
    )
  ).toBeVisible()
  expect(
    screen.queryByText('Request parsing and validation')
  ).not.toBeInTheDocument()
})

test.each([1, undefined])(
  'legacy version %s retains every parse span without guessing which stage it measured',
  async (version) => {
    const legacy = {
      ...(version === undefined ? {} : { schema_version: version }),
      request: { request_parse_ms: 44 },
      business_spans: [44, 29, 29].map((ms, index) => ({
        span_id: `span-${index}`,
        phase: 'request_parse',
        duration_ms: ms,
      })),
    }
    await renderTiming(legacy)
    const stages = within(
      screen.getByRole('region', { name: 'Request stages' })
    )
    expect(
      stages.getByText('Request parsing (legacy, unspecified stage)')
        .nextElementSibling
    ).toHaveTextContent('44 ms')
    const spans = within(
      screen.getByRole('region', { name: 'Processing intervals' })
    )
    expect(
      spans.getAllByText('Request parsing (legacy, unspecified stage)')
    ).toHaveLength(3)
    expect(
      spans.getByText(
        'Legacy parsing timings do not distinguish authentication decoding, business validation, or body decoding.'
      )
    ).toBeVisible()
    expect(
      screen.queryByText('Business request parsing and validation (overall)')
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: 'JSON' }))
    expect(
      JSON.parse(
        screen.getByRole('tabpanel', { name: 'JSON' }).querySelector('pre')
          ?.textContent || '{}'
      )
    ).toEqual(legacy)
  }
)

test('JSON mode preserves null, zero, and unknown fields and switching back restores the explanation', async () => {
  await renderTiming(JSON.stringify(snapshot))
  fireEvent.click(screen.getByRole('tab', { name: 'JSON' }))
  const panel = screen.getByRole('tabpanel', { name: 'JSON' })
  expect(JSON.parse(panel.querySelector('pre')?.textContent || '{}')).toEqual(
    snapshot
  )
  fireEvent.click(screen.getByRole('tab', { name: 'Stage summary' }))
  expect(
    screen.getByText('Before the first upstream request').nextElementSibling
  ).toHaveTextContent('397 s')
})

test('mode selection supports keyboard navigation and exposes the selected tab', async () => {
  const user = userEvent.setup()
  await renderTiming(snapshot)
  screen.getByRole('tab', { name: 'Stage summary' }).focus()
  await user.keyboard('{ArrowRight}{Enter}')
  expect(screen.getByRole('tab', { name: 'JSON' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  expect(screen.getByRole('tabpanel', { name: 'JSON' })).toBeVisible()
})

test('unsupported SDK details, reused connections, retries, and early responses keep distinct explanations', async () => {
  await renderTiming({
    outbound_calls: [
      { call_id: 'opaque', transport_kind: 'sdk_opaque', call_total_ms: 8 },
      {
        call_id: 'retry',
        connection_reused: true,
        missing_reason: 'ambiguous_transport_retry',
      },
      { call_id: 'early', early_response: true },
    ],
  })
  const sdk = within(
    screen.getByRole('region', { name: 'Outbound call opaque' })
  )
  expect(
    sdk.getByText('Sending the upstream request').nextElementSibling
  ).toHaveTextContent('Unsupported by this transport')
  expect(sdk.getByText('Call duration').nextElementSibling).toHaveTextContent(
    '8 ms'
  )
  const retry = within(
    screen.getByRole('region', { name: 'Outbound call retry' })
  )
  expect(retry.getByText('TLS').nextElementSibling).toHaveTextContent(
    'Not applicable'
  )
  expect(
    retry.getByText('Waiting for upstream response headers').nextElementSibling
  ).toHaveTextContent('Multiple writes; duration cannot be paired reliably')
  const early = within(
    screen.getByRole('region', { name: 'Outbound call early' })
  )
  expect(
    early.getByText('Sending the upstream request').nextElementSibling
  ).toHaveTextContent('Response arrived before upload completed')
})

test('incomplete request bodies, invalid metrics and reversed intervals cannot appear as completed zero-duration stages', async () => {
  await renderTiming({
    request: {
      body_state: 'interrupted',
      body_read_ms: 3,
      route_ms: '0',
      auth_ms: -1,
    },
    attempts: [
      { attempt_id: 'bad', attempt_started_at_ms: 20, attempt_ended_at_ms: 10 },
    ],
  })
  expect(
    screen.getByText('Receiving the request body').nextElementSibling
  ).toHaveTextContent('3 ms · Interrupted')
  expect(
    screen.getByText('Channel routing').nextElementSibling
  ).toHaveTextContent('Not recorded')
  expect(
    screen.getByText('Authentication').nextElementSibling
  ).toHaveTextContent('Not recorded')
  expect(
    screen.getByText('Attempt duration').nextElementSibling
  ).toHaveTextContent('Not recorded')
})

test('invalid or unknown timing formats remain inspectable as raw JSON', async () => {
  await renderTiming('unparsed timing payload')
  expect(screen.getByText('No timing summary available')).toBeVisible()
  fireEvent.click(screen.getByRole('tab', { name: 'JSON' }))
  expect(screen.getByRole('tabpanel', { name: 'JSON' })).toHaveTextContent(
    'unparsed timing payload'
  )
})

test('empty timing data offers a translated empty state instead of invented timings', async () => {
  await renderTiming(null)
  expect(screen.getByText('No timing summary available')).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'Copy JSON' })
  ).not.toBeInTheDocument()
})

test.each([
  ['en', '1.23 s'],
  ['zhCN', '1.23 秒'],
  ['zhTW', '1.23 秒'],
  ['fr', '1,23 s'],
  ['ja', '1.23 秒'],
  ['ru', '1,23 с'],
  ['vi', '1,23 giây'],
  ['invalid_locale', '1.23 s'],
])(
  'stage durations format safely in interface language %s',
  async (language, expected) => {
    const i18n = await renderTiming(
      { request: { pre_upstream_ms: 1234.5 } },
      language
    )
    expect(
      screen.getByText(i18n.t('Before the first upstream request'))
        .nextElementSibling
    ).toHaveTextContent(expected)
  }
)

test('changing the interface language updates both stage explanations and number formatting', async () => {
  const i18n = await renderTiming({ request: { pre_upstream_ms: 1234.5 } })
  expect(screen.getByText('1.23 s')).toBeVisible()
  await act(async () => {
    await i18n.changeLanguage('fr')
  })
  expect(screen.getByText('Avant le premier appel en amont')).toBeVisible()
  expect(screen.getByText('1,23 s')).toBeVisible()
})

test.each(['null', '[1,2]', '{"schema_version":99,"other":true}'])(
  'unrecognized structured data %s stays intact in JSON mode',
  async (wire) => {
    await renderTiming(wire)
    expect(screen.getByText('No timing summary available')).toBeVisible()
    fireEvent.click(screen.getByRole('tab', { name: 'JSON' }))
    const panel = screen.getByRole('tabpanel', { name: 'JSON' })
    expect(
      JSON.parse(panel.querySelector('pre')?.textContent || 'false')
    ).toEqual(JSON.parse(wire))
  }
)

test('background settlement snapshots describe their own operation rather than the submission HTTP lifetime', async () => {
  await renderTiming({
    scope: 'background_operation',
    phase: 'consume_log',
    request: { gateway_total_ms: null, pre_upstream_ms: 10 },
    downstream: {},
    coverage: { downstream: 'not_applicable' },
  })
  expect(
    screen.getByRole('region', { name: 'Background operation' })
  ).toBeVisible()
  expect(
    screen.getByText('Operation duration').nextElementSibling
  ).toHaveTextContent('Pending at snapshot')
  expect(screen.queryByText('Request total')).not.toBeInTheDocument()
  expect(
    screen.queryByText('Receiving the request body')
  ).not.toBeInTheDocument()
  expect(screen.queryByText('Downstream delivery')).not.toBeInTheDocument()
})

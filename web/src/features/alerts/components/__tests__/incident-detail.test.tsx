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
import { act, render, screen } from '@testing-library/react'
import i18next, { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'

import { toIntlLocale } from '@/i18n/languages'
import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import { api } from '@/lib/api'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import {
  formatAlertEventSummary,
  formatAlertMoney,
} from '../../lib/event-summary'
import { IncidentDetailDialog } from '../incident-detail'

const incident = {
  id: 9,
  owner_user_id: 0,
  rule_id: 39,
  severity: 'P1',
  target_name: 'usapi-gemini / gemini-3.8-flash',
  status: 'recovered',
  handling_status: 'open',
  close_reason: '',
  opened_at: 1,
}
afterEach(() => vi.restoreAllMocks())

it('shows percentages and USD in stored trigger, repeat and recovery summaries', async () => {
  const translation = createInstance()
  await translation.init({
    lng: 'en',
    fallbackLng: 'en',
    resources: { en, fr },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const events = [
    {
      kind: 'trigger',
      metric: 'request_volume_change',
      value: '50000',
      samples: '6',
      label: 'Call volume change',
      expected: '500%',
    },
    {
      kind: 'repeat',
      metric: 'request_volume_change',
      value: '14000',
      samples: '15',
      label: 'Call volume change',
      expected: '140%',
    },
    {
      kind: 'repeat',
      metric: 'request_volume_change',
      value: '10000',
      samples: '12',
      label: 'Call volume change',
      expected: '100%',
    },
    {
      kind: 'recovery',
      metric: 'request_volume_change',
      value: '0',
      samples: '0',
      label: 'Call volume change',
      expected: '0%',
    },
    {
      kind: 'trigger',
      metric: 'spend_net',
      value: '19770000',
      samples: '3',
      label: 'Net spending',
      expected: '$39.54',
    },
    {
      kind: 'recovery',
      metric: 'balance_remaining',
      value: '500000',
      samples: '1',
      label: 'Remaining balance',
      expected: '$1',
    },
  ]
  vi.spyOn(api, 'request').mockResolvedValue({
    data: {
      success: true,
      data: {
        incident,
        can_manage: false,
        holds: [],
        events: events.map((event, i) => ({
          id: i + 1,
          rule_id: 39,
          incident_id: incident.id,
          severity: 'P1',
          kind: event.kind,
          target_name: incident.target_name,
          summary: `Traffic increase | ${incident.target_name} | ${event.metric}=${event.value} | samples=${event.samples}`,
          evidence: '{}',
          created_at: 1,
          condition_satisfied_at: 1,
        })),
      },
    },
  })
  render(
    <I18nextProvider i18n={translation}>
      <QueryClientProvider client={client}>
        <IncidentDetailDialog
          incident={incident}
          canManage={false}
          onClose={() => {}}
          onEvent={() => {}}
        />
      </QueryClientProvider>
    </I18nextProvider>
  )
  for (const event of events) {
    expect(
      await screen.findByText(
        `Traffic increase | ${incident.target_name} | ${event.label}: ${event.expected} | Valid samples: ${event.samples}`
      )
    ).toBeVisible()
  }
  expect(screen.queryByText(/request_volume_change=/)).not.toBeInTheDocument()
  expect(screen.queryByText(/spend_net=/)).not.toBeInTheDocument()
  await act(async () => {
    await translation.changeLanguage('fr')
  })
  expect(
    screen.getByText(
      `Traffic increase | ${incident.target_name} | ${fr.translation['Call volume change']}: 500% | ${fr.translation['Valid samples']}: 6`
    )
  ).toBeVisible()
  expect(
    screen.getByText(
      `Traffic increase | ${incident.target_name} | ${fr.translation['Net spending']}: 39,54\u00a0$ | ${fr.translation['Valid samples']}: 3`,
      { normalizer: (text) => text }
    )
  ).toBeVisible()
  client.clear()
})

it.each([
  ['19770000', '$39.54'],
  ['0', '$0'],
  ['1', '$0.000002'],
  ['-1', '-$0.000002'],
  ['-19770000', '-$39.54'],
  ['9007199254740991', '18014398509.481982 USD'],
  ['9007199254740993', '18014398509.481986 USD'],
])('formats quota %s as exact USD', (quota, expected) => {
  expect(formatAlertMoney(quota, 'en')).toBe(expected)
})

it('honors a saved quota unit when formatting monetary event summaries', () => {
  expect(
    formatAlertEventSummary(
      'Spending | key2 | spend_net=19770000 | samples=3',
      i18next.getFixedT('en'),
      'en',
      1000000
    )
  ).toBe('Spending | key2 | Net spending: $19.77 | Valid samples: 3')
})

it.each(['CNY', 'TOKENS', 'CUSTOM'] as const)(
  'keeps alert amounts in USD when the general display uses %s',
  (quotaDisplayType) => {
    const original = useSystemConfigStore.getState().config
    try {
      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          displayInCurrency: quotaDisplayType !== 'TOKENS',
          quotaDisplayType,
          usdExchangeRate: 7,
          customCurrencySymbol: '€',
          customCurrencyExchangeRate: 0.9,
        },
      })
      expect(formatAlertMoney('19770000', 'en')).toBe('$39.54')
    } finally {
      useSystemConfigStore.getState().setConfig(original)
    }
  }
)

it.each([
  ['zhCN', '123.45%', '$39.54'],
  ['zhTW', '123.45%', '$39.54'],
  ['en', '123.45%', '$39.54'],
  ['fr', '123,45%', '39,54\u00a0$'],
  ['ja', '123.45%', '$39.54'],
  ['ru', '123,45%', '39,54\u00a0$'],
  ['vi', '123,45%', '39,54\u00a0$'],
  ['invalid_locale', '123.45%', '$39.54'],
])(
  'formats stored summary percentages and dollars for %s',
  (language, expected, dollars) => {
    expect(
      formatAlertEventSummary(
        'Traffic | model | request_volume_change=12345 | samples=6',
        i18next.getFixedT('en'),
        toIntlLocale(language)
      )
    ).toBe(
      `Traffic | model | Call volume change: ${expected} | Valid samples: 6`
    )
    expect(
      formatAlertEventSummary(
        'Spending | key2 | spend_net=19770000 | samples=3',
        i18next.getFixedT('en'),
        toIntlLocale(language)
      )
    ).toBe(`Spending | key2 | Net spending: ${dollars} | Valid samples: 3`)
  }
)

it.each([
  [
    'Rule | model | success_rate=9500 | samples=2',
    'Rule | model | Success rate: 95% | Valid samples: 2',
  ],
  [
    'Rule | model | failure_rate=125 | samples=80',
    'Rule | model | Failure rate: 1.25% | Valid samples: 80',
  ],
  [
    'Rule | model | request_volume_change=900719925474099312 | samples=9007199254740993',
    'Rule | model | Call volume change: 9007199254740993.12% | Valid samples: 9007199254740993',
  ],
  [
    'Rule | success_rate=9500 | samples=2 | model | request_volume_change=50000 | samples=6',
    'Rule | success_rate=9500 | samples=2 | model | Call volume change: 500% | Valid samples: 6',
  ],
  [
    'Rule | model | request_volume_change=500% | samples=6',
    'Rule | model | request_volume_change=500% | samples=6',
  ],
  [
    'Rule | model | request_count=50000 | samples=6',
    'Rule | model | request_count=50000 | samples=6',
  ],
  [
    'Legacy notice | status_code=403, Access denied',
    'Legacy notice | status_code=403, Access denied',
  ],
  ['', ''],
])(
  'preserves summary units, precision and original text in %s',
  (summary, expected) => {
    expect(
      formatAlertEventSummary(summary, i18next.getFixedT('en'), 'en')
    ).toBe(expected)
  }
)

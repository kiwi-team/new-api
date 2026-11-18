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
import { act, render, screen, within } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, expect, it, vi } from 'vitest'

import { toIntlLocale } from '@/i18n/languages'
import { api } from '@/lib/api'

import { buildRule, ruleDefaults } from '../../lib/rule-form'
import type { AlertRule, RuleState } from '../../types'
import { RuleDetailDialog } from '../rule-detail'

const languages = ['zhCN', 'zhTW', 'en', 'fr', 'ja', 'ru', 'vi']

async function fixture(
  metric = 'success_rate',
  language = 'en',
  states?: RuleState[]
) {
  const translation = createInstance()
  await translation.use(initReactI18next).init({
    lng: language,
    fallbackLng: 'en',
    resources: Object.fromEntries(
      languages.map((lang) => [lang, { translation: { Value: 'Value' } }])
    ),
  })
  const config = buildRule({
    ...ruleDefaults(undefined, 1),
    name: 'Channel health',
    scope: 'channel',
    entities: ['49'],
    metric,
    threshold: '95',
    recoveryThreshold: '99',
  })
  config.quota_unit = 1000000
  const state = {
    id: 'sampled',
    target_name: 'Sampled channel',
    status: 'normal',
    data_status: 'warming_up',
    last_value: '10000',
    last_count: '1',
    model_key: '',
  }
  const rule: AlertRule = {
    id: 5,
    owner_user_id: 0,
    revision: 1,
    effective_at: 1,
    config,
    targets: [],
    states: states ?? [
      state,
      {
        ...state,
        id: 'zero',
        target_name: 'Zero-valued channel',
        data_status: 'complete',
        last_value: '0',
        last_count: '5',
      },
      {
        ...state,
        id: 'empty',
        target_name: 'No-sample channel',
        last_value: '0',
        last_count: '0',
      },
      {
        ...state,
        id: 'decimal',
        target_name: 'Decimal channel',
        last_value: '1234',
        last_count: '3',
      },
    ],
  }
  vi.spyOn(api, 'request').mockResolvedValue({
    data: { success: true, data: rule },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <I18nextProvider i18n={translation}>
      <QueryClientProvider client={client}>
        <RuleDetailDialog rule={rule} onClose={() => {}} />
      </QueryClientProvider>
    </I18nextProvider>
  )
  return { client, translation }
}

afterEach(() => vi.restoreAllMocks())

it('shows percentages in rule detail and distinguishes zero rates from absent samples', async () => {
  const { client } = await fixture()
  const sampled = await screen.findByRole('row', { name: /Sampled channel/ })
  expect(within(sampled).getByRole('cell', { name: '100%' })).toBeVisible()
  expect(
    within(sampled).queryByRole('cell', { name: '10000' })
  ).not.toBeInTheDocument()
  expect(within(sampled).getByText('Warming up')).toBeVisible()
  const zero = screen.getByRole('row', { name: /Zero-valued channel/ })
  expect(within(zero).getByRole('cell', { name: '0%' })).toBeVisible()
  const empty = screen.getByRole('row', { name: /No-sample channel/ })
  expect(within(empty).getByRole('cell', { name: '—' })).toBeVisible()
  expect(
    within(empty).queryByRole('cell', { name: '0%' })
  ).not.toBeInTheDocument()
  client.clear()
})

it.each([...languages, 'invalid_locale'])(
  'formats detail percentages for %s and responds to language changes',
  async (language) => {
    const { client, translation } = await fixture('success_rate', language)
    const row = await screen.findByRole('row', { name: /Decimal channel/ })
    const number = new Intl.NumberFormat(toIntlLocale(language), {
      maximumFractionDigits: 2,
    }).format(12.34)
    expect(within(row).getByRole('cell', { name: `${number}%` })).toBeVisible()
    await act(async () => {
      await translation.changeLanguage('fr')
    })
    expect(within(row).getByRole('cell', { name: '12,34%' })).toBeVisible()
    client.clear()
  }
)

it('keeps ordinary counts exact without converting them into percentages', async () => {
  const { client } = await fixture('request_count', 'en', [
    {
      id: 'count',
      target_name: 'High request count',
      status: 'normal',
      data_status: 'complete',
      last_value: '9007199254740993',
      last_count: '9007199254740995',
      model_key: '',
    },
  ])
  const row = await screen.findByRole('row', { name: /High request count/ })
  expect(
    within(row).getByRole('cell', { name: '9007199254740993' })
  ).toBeVisible()
  expect(
    within(row).getByRole('cell', { name: '9007199254740995' })
  ).toBeVisible()
  expect(within(row).queryByText(/%/)).not.toBeInTheDocument()
  client.clear()
})

it('uses the saved quota unit to show monetary detail values in USD', async () => {
  const { client } = await fixture('spend_net', 'en', [
    {
      id: 'spend',
      target_name: 'Key spending',
      status: 'normal',
      data_status: 'complete',
      last_value: '19770000',
      last_count: '3',
      model_key: '',
    },
  ])
  expect(await screen.findByRole('cell', { name: '$19.77' })).toBeVisible()
  client.clear()
})

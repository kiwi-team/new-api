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
import { expect, it } from 'vitest'

import { buildRule, ruleDefaults } from '../../lib/rule-form'
import { EvidenceSummary } from '../evidence-summary'

it('shows the original legacy notice as text without inventing a numeric measurement', () => {
  const config = buildRule({
    ...ruleDefaults(undefined, 1),
    name: 'Imported notice',
    metric: 'legacy_notice',
    legacySignal: 'user_notifications',
  })
  const content = 'Budget remaining: $10\n<img src=x onerror=alert(1)>'
  render(
    <EvidenceSummary
      config={config}
      evidence={JSON.stringify({ title: 'Budget', content })}
    />
  )
  expect(screen.getByRole('note')).toHaveTextContent('Budget remaining: $10')
  expect(screen.getByRole('note')).toHaveTextContent(
    '<img src=x onerror=alert(1)>'
  )
  expect(screen.queryByRole('img')).not.toBeInTheDocument()
})

it('shows the current traffic and frozen baseline with a percentage instead of basis points', () => {
  const config = buildRule({
    ...ruleDefaults(undefined, 1),
    name: 'Traffic drop',
    metric: 'request_volume_change',
    baselineMode: 'manual',
    baselineCount: 200,
    changeDirection: 'decrease',
    threshold: '50',
    recoveryThreshold: '20',
  })
  render(
    <EvidenceSummary
      config={config}
      evidence={JSON.stringify({
        measurement: {
          value: '6000',
          samples: '80',
          status: 'complete',
          window_start: 1700000000000,
          window_end: 1700000300000,
          request_ids: [],
          traffic: {
            count: '80',
            baseline_scaled: '2000000',
            change_bps: '-6000',
            baseline_start: 0,
            baseline_end: 0,
            frozen: true,
          },
        },
      })}
    />
  )
  expect(screen.getByText('Call volume change: 60%')).toBeVisible()
  expect(screen.getByText('Started calls: 80')).toBeVisible()
  expect(screen.getByText('Baseline calls per window: 200')).toBeVisible()
  expect(
    screen.getByText(
      'Recovery uses the baseline frozen when the alert condition first matched.'
    )
  ).toBeVisible()
  expect(screen.queryByText(/6000/)).not.toBeInTheDocument()
})

it('shows monetary evidence in USD using the saved quota conversion unit', () => {
  const config = buildRule({
    ...ruleDefaults(undefined, 1),
    name: 'Key spending',
    metric: 'spend_net',
  })
  config.quota_unit = 1000000
  render(
    <EvidenceSummary
      config={config}
      evidence={JSON.stringify({
        measurement: {
          value: '19770000',
          samples: '3',
          status: 'complete',
          window_start: 1700000000000,
          window_end: 1700003600000,
          request_ids: [],
        },
      })}
    />
  )
  expect(screen.getByText('Net spending: $19.77')).toBeVisible()
  expect(screen.queryByText(/19770000/)).not.toBeInTheDocument()
})

it.each([
  ['success_rate', 'Success rate', '0', '0', '—'],
  ['failure_rate', 'Failure rate', '0', '5', '0%'],
  ['request_volume_change', 'Call volume change', '10000', '0', '100%'],
])(
  'distinguishes unavailable %s evidence from a valid zero call count',
  (metric, label, value, samples, expected) => {
    const config = buildRule({ ...ruleDefaults(undefined, 1), metric })
    render(
      <EvidenceSummary
        config={config}
        evidence={JSON.stringify({
          measurement: {
            value,
            samples,
            status: 'complete',
            window_start: 1,
            window_end: 2,
            request_ids: [],
          },
        })}
      />
    )
    expect(screen.getByText(`${label}: ${expected}`)).toBeVisible()
  }
)

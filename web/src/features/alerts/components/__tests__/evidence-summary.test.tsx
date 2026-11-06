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

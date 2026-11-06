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
import { describe, expect, it } from 'vitest'

import {
  buildRule,
  metricAllowed,
  ruleDefaults,
  ruleFormSchema,
} from '../rule-form'
import { ruleTemplates } from '../templates'

describe('alert rule form contract', () => {
  it('keeps in-flight rules notification-only and independent of terminal windows', () => {
    const values = {
      ...ruleDefaults(undefined, 1),
      name: 'Output wait',
      scope: 'channel_model' as const,
      metric: 'inflight_overdue_count',
      threshold: '1',
      entities: ['41'],
      models: ['model-a'],
      action: 'disable_channel_model',
    }
    const rule = buildRule(values)
    expect(rule.actions).toEqual([])
    expect(rule.condition).toMatchObject({
      threshold_count: 1,
      recovery_threshold_count: 0,
      min_samples: 0,
      window: { type: '', duration_seconds: 0 },
    })
    expect(rule.scope).toMatchObject({
      channel_ids: [41],
      model_selector: { models: ['model-a'] },
      group_mode: 'per_entity',
    })
  })
  it('preserves exact decimal USD input without putting fractional numbers in count fields', () => {
    const values = {
      ...ruleDefaults(undefined, 1),
      metric: 'spend_net',
      threshold: '0.12345678',
      recoveryThreshold: '0.05',
    }
    const rule = buildRule(values)
    expect(rule.condition.threshold_usd).toBe('0.12345678')
    expect(rule.condition.threshold_count).toBe(0)
    expect(rule.condition.threshold_ms).toBe(0)
  })
  it('does not offer platform or channel-only metrics for personal key rules', () => {
    expect(metricAllowed('inflight_overdue_count', 'api_key')).toBe(false)
    expect(metricAllowed('source_lag', 'account')).toBe(false)
    expect(metricAllowed('consecutive_failures', 'api_key')).toBe(false)
    expect(metricAllowed('duration_ms', 'api_key')).toBe(true)
  })
  it('restores editable defaults for an in-flight rule whose hidden window fields are empty', () => {
    const original = buildRule({
      ...ruleDefaults(undefined, 1),
      name: 'In-flight',
      scope: 'channel_model',
      metric: 'inflight_overdue_count',
      threshold: '1',
    })
    const values = ruleDefaults({
      id: 1,
      owner_user_id: 0,
      revision: 1,
      effective_at: 0,
      config: original,
    })
    expect(ruleFormSchema.safeParse(values).success).toBe(true)
  })
  it('keeps subscription and wallet balances separate and requires an explicit subscription', () => {
    const values = {
      ...ruleDefaults(undefined, 1),
      name: 'Budget',
      scope: 'account' as const,
      metric: 'balance_remaining',
      balancePool: 'subscription',
      subscription: '8',
    }
    expect(buildRule(values).condition).toMatchObject({
      balance_pool: 'subscription',
      subscription_id: 8,
    })
    expect(
      ruleFormSchema.safeParse({ ...values, subscription: '' }).success
    ).toBe(false)
  })
  it('requires an explicit recovery protocol instead of guessing from the channel name', () => {
    const values = {
      ...ruleDefaults(undefined, 1),
      name: 'Recovery',
      scope: 'channel_model' as const,
      action: 'disable_channel_model',
      autoRecovery: true,
    }
    expect(ruleFormSchema.safeParse(values).success).toBe(false)
    expect(
      ruleFormSchema.safeParse({ ...values, recoveryProtocol: 'gemini' })
        .success
    ).toBe(true)
  })
  it('templates require user-selected objects and do not preauthorize actions', () => {
    for (const template of ruleTemplates) {
      const rule = buildRule({
        ...ruleDefaults(undefined, 1),
        ...template.values,
        name: template.name,
      })
      expect(rule.scope.entity_ids).toEqual([])
      expect(rule.scope.channel_ids).toEqual([])
      expect(rule.actions).toEqual([])
      expect(rule.enabled).toBe(false)
    }
  })
})

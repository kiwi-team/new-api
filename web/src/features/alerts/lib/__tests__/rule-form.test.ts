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

import { toIntlLocale } from '@/i18n/languages'

import type { AlertRule } from '../../types'
import {
  buildRule,
  formatAlertMetricValue,
  canUseRuleTarget,
  metricAllowed,
  metricDefaults,
  ruleDefaults,
  ruleFormSchema,
} from '../rule-form'
import { ruleTemplates } from '../templates'

describe('alert rule form contract', () => {
  it('lets Root select targets from every user while ordinary users remain isolated', () => {
    const target = {
      id: 91,
      owner_user_id: 2,
      name: 'Other account operations',
      type: 'email',
      enabled: true,
      revision: 1,
      language: 'en',
      address_hint: '',
      last_test_status: '',
    }
    const values = { ...ruleDefaults(undefined, 1), scope: 'api_key' as const }
    expect(canUseRuleTarget(target, values, true)).toBe(true)
    expect(canUseRuleTarget(target, values, false)).toBe(false)
  })
  it('preserves saved guard, probe, notification, report and acknowledgement policy when renaming', () => {
    const config = buildRule({
      ...ruleDefaults(undefined, 1),
      name: 'Channel protection',
      scope: 'channel',
      entities: ['49'],
      targets: ['16'],
      action: 'disable_channel',
      autoRecovery: true,
      recoveryProtocol: 'openai',
    })
    config.quota_unit = 500000
    config.notify.on_recovery = false
    config.action_guard = {
      min_remaining_channels: 3,
      on_no_alternative: 'notify_only',
    }
    config.recovery = {
      ...config.recovery,
      successes: 5,
      observe_seconds: 300,
      models: ['dedicated-probe-model'],
    }
    config.trigger.report = {
      include_zero: false,
      send_revision: true,
      allow_first_partial: true,
      backfill_periods: 2,
      partial_after_seconds: 3600,
    }
    config.acknowledgement.remind_after_seconds = 60
    config.acknowledgement.escalate_after_seconds = 120
    const rule: AlertRule = {
      id: 1,
      owner_user_id: 0,
      revision: 2,
      effective_at: 1,
      config,
    }
    const values = ruleFormSchema.parse({
      ...ruleDefaults(rule, 1),
      name: 'Renamed protection',
    })
    const saved = buildRule(values)
    expect(saved.name).toBe('Renamed protection')
    expect(saved.quota_unit).toBe(config.quota_unit)
    expect(saved.notify).toEqual(config.notify)
    expect(saved.action_guard).toEqual(config.action_guard)
    expect(saved.recovery).toEqual(config.recovery)
    expect(saved.trigger).toEqual(config.trigger)
    expect(saved.acknowledgement).toEqual(config.acknowledgement)
    expect(saved.scope.model_selector.models).toEqual([])
  })

  it('keeps non-applicable zero policy values and stops probes when actions are removed', () => {
    const values = {
      ...ruleDefaults(undefined, 1),
      name: 'Manual recovery',
      scope: 'channel' as const,
      entities: ['49'],
      targets: ['16'],
      minRemainingChannels: 0,
      recoverySuccesses: 0,
      recoveryObserveSeconds: 0,
      remindAfterSeconds: 0,
      escalateAfterSeconds: 0,
      reportBackfillPeriods: 0,
      reportPartialAfterSeconds: 0,
      notifyRecovery: false,
    }
    const config = buildRule(values)
    const rule: AlertRule = {
      id: 1,
      owner_user_id: 0,
      revision: 1,
      effective_at: 1,
      config,
    }
    const defaults = ruleDefaults(rule, 1)
    expect(defaults).toMatchObject({
      minRemainingChannels: 0,
      recoverySuccesses: 0,
      recoveryObserveSeconds: 0,
      remindAfterSeconds: 0,
      escalateAfterSeconds: 0,
      reportBackfillPeriods: 0,
      reportPartialAfterSeconds: 0,
      notifyRecovery: false,
    })
    expect(
      buildRule({ ...defaults, autoRecovery: true, action: '' }).recovery.mode
    ).toBe('manual')
  })

  it.each(['zhCN', 'zhTW', 'en', 'fr', 'ja', 'ru', 'vi', 'invalid_locale'])(
    'formats baseline percentages for %s without exposing basis points',
    (language) => {
      const locale = toIntlLocale(language)
      expect(
        formatAlertMetricValue('request_volume_change', '15000', locale)
      ).toBe('150%')
      expect(formatAlertMetricValue('success_rate', '10000', locale)).toBe(
        '100%'
      )
      expect(
        formatAlertMetricValue(
          'request_started_count',
          '9007199254740993',
          locale
        )
      ).toBe('9007199254740993')
      expect(
        formatAlertMetricValue(
          'request_volume_change',
          '900719925474099312',
          locale
        )
      ).toBe('9007199254740993.12%')
    }
  )
  it.each([
    ['success_rate', '0', '0', 'warming_up', '—'],
    ['failure_rate', '0', '4', 'complete', '0%'],
    ['success_rate', '10000', '1', 'warming_up', '100%'],
    ['budget_remaining_ratio', '10000', '0', 'complete', '100%'],
    ['request_volume_change', '10000', '0', 'complete', '100%'],
    ['request_volume_change', '0', '0', 'warming_up', '—'],
    ['request_count', '0', '0', 'complete', '0'],
  ])(
    'formats %s with value %s and %s samples (%s) as %s',
    (metric, value, samples, status, expected) => {
      expect(
        formatAlertMetricValue(metric, value, 'en', { samples, status })
      ).toBe(expected)
    }
  )
  it.each([
    {
      scope: 'channel' as const,
      owner: 1,
      targetOwner: 1,
      enforced: false,
      root: true,
      allowed: true,
    },
    {
      scope: 'channel' as const,
      owner: 1,
      targetOwner: 0,
      enforced: false,
      root: true,
      allowed: true,
    },
    {
      scope: 'channel' as const,
      owner: 1,
      targetOwner: 0,
      enforced: false,
      root: false,
      allowed: false,
    },
    {
      scope: 'api_key' as const,
      owner: 1,
      targetOwner: 1,
      enforced: false,
      root: false,
      allowed: true,
    },
    {
      scope: 'api_key' as const,
      owner: 1,
      targetOwner: 2,
      enforced: true,
      root: true,
      allowed: true,
    },
    {
      scope: 'api_key' as const,
      owner: 1,
      targetOwner: 0,
      enforced: false,
      root: true,
      allowed: true,
    },
    {
      scope: 'api_key' as const,
      owner: 1,
      targetOwner: 0,
      enforced: true,
      root: false,
      allowed: false,
    },
    {
      scope: 'api_key' as const,
      owner: 1,
      targetOwner: 0,
      enforced: true,
      root: true,
      allowed: true,
    },
  ])(
    'matches rule target ownership for $scope, owner $owner, target owner $targetOwner, enforced $enforced and root $root',
    (input) => {
      expect(
        canUseRuleTarget(
          {
            id: 6,
            name: 'Feishu',
            owner_user_id: input.targetOwner,
            type: 'feishu',
            enabled: true,
            revision: 1,
            language: 'en',
            address_hint: '',
            last_test_status: '',
          },
          input,
          input.root
        )
      ).toBe(input.allowed)
    }
  )

  it.each([
    { targets: [], action: '', valid: false },
    { targets: ['16'], action: '', valid: true },
    { targets: [], action: 'disable_api_key', valid: true },
  ])(
    'requires a notification target or action ($valid)',
    ({ targets, action, valid }) => {
      const result = ruleFormSchema.safeParse({
        ...ruleDefaults(undefined, 1),
        name: 'Key health',
        targets,
        action,
      })
      expect(result.success).toBe(valid)
      if (!result.success) {
        expect(result.error.issues).toEqual([
          expect.objectContaining({
            path: ['targets'],
            message:
              'Select at least one notification target or automatic action.',
          }),
        ])
      }
    }
  )

  it('requires a destination for in-flight rules even when a stale action value is present', () => {
    const result = ruleFormSchema.safeParse({
      ...ruleDefaults(undefined, 1),
      name: 'Output wait',
      metric: 'inflight_overdue_count',
      action: 'disable_api_key',
    })
    expect(result.success).toBe(false)
    if (!result.success) {
      expect(result.error.issues).toEqual([
        expect.objectContaining({ path: ['targets'] }),
      ])
    }
  })

  it.each([
    'success_rate',
    'failure_rate',
    'timeout_rate',
    'slow_request_rate',
    'budget_remaining_ratio',
  ])('edits %s in percentages and preserves stored basis points', (metric) => {
    const values = {
      ...ruleDefaults(undefined, 1),
      name: 'Percentage rule',
      metric,
      threshold: '80.01',
      recoveryThreshold: '99.99',
    }
    const config = buildRule(values)
    expect(config.condition).toMatchObject({
      threshold_bps: 8001,
      recovery_threshold_bps: 9999,
      threshold_ms: 0,
      threshold_count: 0,
      threshold_usd: '',
    })
    config.condition.threshold_bps = 8001
    config.condition.recovery_threshold_bps = 9999
    const restored = ruleDefaults({
      id: 1,
      owner_user_id: 1,
      revision: 1,
      effective_at: 0,
      config,
    })
    expect(restored).toMatchObject({
      threshold: '80.01',
      recoveryThreshold: '99.99',
    })
    expect(buildRule(restored)).toEqual(config)
  })

  it.each(['0', '100', '0.01', '80.01'])(
    'accepts percentage boundary and decimal input %s without losing precision',
    (percentage) => {
      const values = {
        ...ruleDefaults(undefined, 1),
        name: 'Percentage rule',
        metric: 'success_rate',
        threshold: percentage,
        recoveryThreshold: percentage,
        targets: ['7'],
      }
      expect(ruleFormSchema.safeParse(values).success).toBe(true)
      const config = buildRule(values)
      expect(
        ruleDefaults({
          id: 1,
          owner_user_id: 1,
          revision: 1,
          effective_at: 0,
          config,
        })
      ).toMatchObject({ threshold: percentage, recoveryThreshold: percentage })
    }
  )

  it.each(['-1', '100.01', '80.001', '', '1e2'])(
    'rejects percentage input %s for both thresholds',
    (percentage) => {
      const result = ruleFormSchema.safeParse({
        ...ruleDefaults(undefined, 1),
        name: 'Percentage rule',
        metric: 'success_rate',
        threshold: percentage,
        recoveryThreshold: percentage,
        targets: ['7'],
      })
      expect(result.success).toBe(false)
      if (result.success) return
      expect(result.error.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({ path: ['threshold'] }),
          expect.objectContaining({ path: ['recoveryThreshold'] }),
        ])
      )
    }
  )

  it.each([
    { operator: 'lt' as const, threshold: '100', recovery: '99', valid: false },
    { operator: 'lt' as const, threshold: '100', recovery: '100', valid: true },
    { operator: 'lte' as const, threshold: '95', recovery: '95', valid: false },
    {
      operator: 'lte' as const,
      threshold: '95',
      recovery: '95.01',
      valid: true,
    },
    { operator: 'gt' as const, threshold: '5', recovery: '6', valid: false },
    { operator: 'gt' as const, threshold: '5', recovery: '5', valid: true },
    { operator: 'gte' as const, threshold: '5', recovery: '5', valid: false },
    { operator: 'gte' as const, threshold: '5', recovery: '4.99', valid: true },
  ])(
    'validates recovery for $operator $threshold with recovery $recovery (valid: $valid)',
    ({ operator, threshold, recovery, valid }) => {
      const result = ruleFormSchema.safeParse({
        ...ruleDefaults(undefined, 1),
        name: 'Channel success rate',
        targets: ['7'],
        metric: 'success_rate',
        operator,
        threshold,
        recoveryThreshold: recovery,
      })
      expect(result.success).toBe(valid)
      if (!result.success) {
        expect(result.error.issues).toEqual([
          expect.objectContaining({ path: ['recoveryThreshold'] }),
        ])
      }
    }
  )

  it('does not apply incident recovery ordering to a one-time percentage rule', () => {
    expect(
      ruleFormSchema.safeParse({
        ...ruleDefaults(undefined, 1),
        name: 'One-time channel notification',
        targets: ['7'],
        metric: 'success_rate',
        operator: 'lt',
        threshold: '100',
        recoveryThreshold: '99',
        trigger: 'once',
      }).success
    ).toBe(true)
  })

  it('uses percentages in metric defaults and preserves the failure template thresholds', () => {
    expect(metricDefaults('success_rate')).toMatchObject({
      threshold: '95',
      recoveryThreshold: '99',
    })
    expect(metricDefaults('failure_rate')).toMatchObject({
      threshold: '5',
      recoveryThreshold: '2',
    })
    expect(metricDefaults('budget_remaining_ratio')).toMatchObject({
      threshold: '20',
      recoveryThreshold: '30',
    })
    const template = ruleTemplates.find(
      (item) => item.name === 'Channel model failures'
    )
    expect(template).toBeDefined()
    expect(
      buildRule({ ...ruleDefaults(undefined, 1), ...template?.values })
        .condition
    ).toMatchObject({ threshold_bps: 5000, recovery_threshold_bps: 1000 })
  })

  it('keeps count thresholds above 100 unchanged', () => {
    const values = {
      ...ruleDefaults(undefined, 1),
      name: 'Request volume',
      targets: ['7'],
      metric: 'request_count',
      threshold: '200',
      recoveryThreshold: '250',
    }
    expect(ruleFormSchema.safeParse(values).success).toBe(true)
    expect(buildRule(values).condition).toMatchObject({
      threshold_count: 200,
      recovery_threshold_count: 250,
      threshold_bps: 0,
    })
  })

  it.each([null, undefined])(
    'normalizes null API collections and %s scope selectors when editing a saved rule',
    (selector) => {
      const config = buildRule({
        ...ruleDefaults(undefined, 1),
        name: 'Imported rule',
        scope: 'account',
      })
      const rule: AlertRule = {
        id: 1,
        owner_user_id: 1,
        revision: 1,
        effective_at: 0,
        config: {
          ...config,
          actions: null,
          scope: {
            ...config.scope,
            channel_ids: selector,
            entity_ids: selector,
            model_selector: { ...config.scope.model_selector, models: null },
          },
          notify: { ...config.notify, target_ids: null },
          acknowledgement: {
            ...config.acknowledgement,
            escalation_target_ids: null,
          },
        },
      }
      const values = ruleDefaults(rule)
      expect(values).toMatchObject({
        name: 'Imported rule',
        owner: 1,
        targets: [],
        entities: [],
        models: [],
        escalationTargets: [],
        action: '',
      })
      expect(
        ruleFormSchema.safeParse({ ...values, targets: ['7'] }).success
      ).toBe(true)
      expect(buildRule(values)).toMatchObject({
        actions: [],
        notify: { target_ids: [] },
        acknowledgement: { escalation_target_ids: [] },
      })
    }
  )

  it('preserves configured actions, destinations and selectors when restoring a saved rule', () => {
    const original = buildRule({
      ...ruleDefaults(undefined, 1),
      name: 'Key latency',
      scope: 'api_key',
      entities: ['11'],
      targets: ['7'],
      escalationTargets: ['9'],
      models: ['model-a'],
      action: 'disable_api_key',
    })
    const values = ruleDefaults({
      id: 1,
      owner_user_id: 1,
      revision: 1,
      effective_at: 0,
      config: original,
    })
    expect(values).toMatchObject({
      entities: ['11'],
      targets: ['7'],
      escalationTargets: ['9'],
      models: ['model-a'],
      action: 'disable_api_key',
    })
    expect(buildRule(values)).toEqual(original)
  })

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
      targets: ['7'],
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
      targets: ['7'],
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

it('serializes a manual traffic baseline and allows increases above 100 percent', () => {
  const values = {
    ...ruleDefaults(),
    ...metricDefaults('request_volume_change'),
    scope: 'model' as const,
    owner: 0,
    enforced: true,
    name: 'Global traffic',
    models: ['gemini-3.8-flash'],
    targets: ['16'],
    baselineMode: 'manual' as const,
    baselineCount: 200,
    changeDirection: 'increase' as const,
    threshold: '150',
    recoveryThreshold: '20',
  }
  expect(ruleFormSchema.safeParse(values).success).toBe(true)
  const config = buildRule(values)
  expect(config.condition).toMatchObject({
    metric: 'request_volume_change',
    threshold_bps: 15000,
    recovery_threshold_bps: 2000,
    baseline: { mode: 'manual', count: 200 },
    change_direction: 'increase',
    min_samples: 0,
  })
  expect(config.scope.type).toBe('model')
  const edited = ruleDefaults({
    id: 1,
    owner_user_id: 0,
    revision: 1,
    effective_at: 1,
    config,
  })
  expect(edited.baselineCount).toBe(200)
  expect(edited.threshold).toBe('150')
  expect(
    ruleFormSchema.safeParse({ ...values, changeDirection: 'decrease' }).success
  ).toBe(false)
})
it('rejects overlapping history settings and leaves ordinary rules without a baseline', () => {
  const values = {
    ...ruleDefaults(),
    ...metricDefaults('request_volume_change'),
    name: 'Traffic',
    targets: ['16'],
    baselineMode: 'historical' as const,
    baselineMinutes: 1,
    minutes: 5,
  }
  expect(ruleFormSchema.safeParse(values).success).toBe(false)
  const ordinary = buildRule({
    ...ruleDefaults(),
    ...metricDefaults('request_count'),
    samples: 20,
  })
  expect(ordinary.condition.baseline).toBeUndefined()
  expect(ordinary.condition.min_samples).toBe(0)
})

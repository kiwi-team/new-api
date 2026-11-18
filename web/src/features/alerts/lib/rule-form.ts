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
import { z } from 'zod'

import { formatNumber } from '@/lib/format'

import type { AlertRule, AlertScope, AlertTarget, RuleSpec } from '../types'

export function isPercentageMetric(metric: string): boolean {
  return (
    metric.endsWith('_rate') ||
    metric === 'budget_remaining_ratio' ||
    metric === 'request_volume_change'
  )
}

export function formatAlertMetricValue(
  metric: string,
  value: string,
  locale?: Intl.LocalesArgument,
  measurement?: { samples?: string; status?: string }
): string {
  // Request rates need a denominator. Budget ratios and a complete traffic
  // drop still have meaningful percentages when the current call count is zero.
  if (metric.endsWith('_rate') && measurement?.samples === '0') return '—'
  if (
    metric === 'request_volume_change' &&
    measurement?.status !== undefined &&
    measurement.status !== 'complete'
  ) {
    return '—'
  }
  if (!isPercentageMetric(metric)) return value
  if (Number.isSafeInteger(Number(value))) {
    return `${formatNumber(Number(value) / 100, locale)}%`
  }
  if (!/^-?\d+$/.test(value)) return value
  const bps = BigInt(value)
  const magnitude = bps < 0n ? -bps : bps
  const fraction = (magnitude % 100n)
    .toString()
    .padStart(2, '0')
    .replace(/0+$/, '')
  return `${bps < 0n ? '-' : ''}${magnitude / 100n}${fraction ? `.${fraction}` : ''}%`
}

export function isPlatformScope(scope: AlertScope): boolean {
  return [
    'channel',
    'channel_model',
    'model',
    'system',
    'uid_budget',
    'project_budget',
  ].includes(scope)
}

export const ruleFormSchema = z
  .object({
    name: z.string().trim().min(1).max(160),
    quotaUnit: z.number().int().nonnegative().optional(),
    scope: z.enum([
      'account',
      'api_key',
      'channel',
      'channel_model',
      'model',
      'system',
      'uid_budget',
      'project_budget',
    ]),
    baselineMode: z.enum(['manual', 'historical']),
    baselineCount: z.number().int().min(1).max(1000000000),
    baselineMinutes: z.number().int().min(1).max(1440),
    baselineMinimumCount: z.number().int().min(1).max(1000000000),
    changeDirection: z.enum(['increase', 'decrease']),
    balancePool: z.string(),
    subscription: z.string(),
    taskType: z.string(),
    legacySignal: z.string().optional(),
    metric: z.string().min(1),
    operator: z.enum(['gt', 'gte', 'lt', 'lte']),
    threshold: z.string().regex(/^\d+(\.\d+)?$/),
    recoveryThreshold: z.string().regex(/^\d+(\.\d+)?$/),
    window: z.string(),
    minutes: z.number().int().min(1).max(1440),
    samples: z.number().int().min(0).max(100000),
    hold: z.number().int().min(0).max(86400),
    severity: z.string(),
    aggregation: z.string(),
    deadline: z.string(),
    deadlineSeconds: z.number().int().min(10).max(86400),
    slowSeconds: z.number().int().min(1).max(86400),
    trigger: z.string(),
    targets: z.array(z.string()),
    entities: z.array(z.string()),
    models: z.array(z.string()),
    groupMode: z.string(),
    allOwned: z.boolean(),
    allModels: z.boolean(),
    futureModels: z.boolean(),
    reportZero: z.boolean(),
    reportRevision: z.boolean(),
    reportFirstPartial: z.boolean(),
    reportBackfillPeriods: z.number().int(),
    reportPartialAfterSeconds: z.number().int(),
    notifyRecovery: z.boolean(),
    includeFuture: z.boolean(),
    enabled: z.boolean(),
    action: z.string(),
    noAlternative: z.string(),
    minRemainingChannels: z.number().int(),
    autoRecovery: z.boolean(),
    sendAt: z.string(),
    timezone: z.string(),
    stepUSD: z.string(),
    recoveryHold: z.number().int().min(0).max(86400),
    recoveryDuration: z.number().int().min(0).max(86400),
    recoveryProtocol: z.string(),
    recoveryTTFT: z.number().int().min(0).max(86400),
    recoverySuccesses: z.number().int(),
    recoveryObserveSeconds: z.number().int(),
    recoveryModels: z.array(z.string()),
    protocol: z.string(),
    stream: z.string(),
    owner: z.number().int().min(0),
    enforced: z.boolean(),
    repeat: z.number().int().min(60).max(604800),
    acknowledge: z.boolean(),
    remindAfterSeconds: z.number().int(),
    escalateAfterSeconds: z.number().int(),
    escalationTargets: z.array(z.string()),
  })
  .superRefine((values, ctx) => {
    if (
      values.targets.length === 0 &&
      (!values.action || values.metric === 'inflight_overdue_count')
    ) {
      ctx.addIssue({
        code: 'custom',
        path: ['targets'],
        message: 'Select at least one notification target or automatic action.',
      })
    }
    if (
      values.metric === 'request_volume_change' &&
      values.baselineMode === 'historical' &&
      values.baselineMinutes < values.minutes
    ) {
      ctx.addIssue({
        code: 'custom',
        path: ['baselineMinutes'],
        message: 'Baseline history must be at least the statistics window.',
      })
    }
    if (isPercentageMetric(values.metric)) {
      const volumeChange = values.metric === 'request_volume_change'
      const maximum =
        volumeChange && values.changeDirection === 'increase' ? 1000000 : 100
      let validPercentages = true
      for (const field of ['threshold', 'recoveryThreshold'] as const) {
        if (
          !/^\d+(\.\d{1,2})?$/.test(values[field]) ||
          Number(values[field]) > maximum
        ) {
          validPercentages = false
          ctx.addIssue({
            code: 'custom',
            path: [field],
            message: volumeChange
              ? 'Enter a valid change percentage with up to two decimal places.'
              : 'Enter a percentage from 0 to 100 with up to two decimal places.',
          })
        }
      }
      if (validPercentages && values.trigger === 'incident') {
        const threshold = Number(values.threshold)
        const recovery = Number(values.recoveryThreshold)
        let message = ''
        if (volumeChange && threshold <= 0) {
          ctx.addIssue({
            code: 'custom',
            path: ['threshold'],
            message: 'Change threshold must be greater than zero.',
          })
        }
        if (volumeChange && recovery >= threshold) {
          message = 'Recovery threshold must be less than the alert threshold.'
        } else if (values.operator === 'lt' && recovery < threshold) {
          message = 'Recovery threshold must be at least the alert threshold.'
        } else if (values.operator === 'lte' && recovery <= threshold) {
          message =
            'Recovery threshold must be greater than the alert threshold.'
        } else if (values.operator === 'gt' && recovery > threshold) {
          message = 'Recovery threshold must be at most the alert threshold.'
        } else if (values.operator === 'gte' && recovery >= threshold) {
          message = 'Recovery threshold must be less than the alert threshold.'
        }
        if (message) {
          ctx.addIssue({ code: 'custom', path: ['recoveryThreshold'], message })
        }
      }
    }
    if (
      values.autoRecovery &&
      values.action &&
      ['channel', 'channel_model'].includes(values.scope) &&
      !['openai', 'claude', 'gemini', 'openai_responses'].includes(
        values.recoveryProtocol
      )
    ) {
      ctx.addIssue({
        code: 'custom',
        path: ['recoveryProtocol'],
        message: 'Required',
      })
    }
    if (
      values.metric === 'balance_remaining' &&
      values.balancePool === 'subscription' &&
      (!Number.isSafeInteger(Number(values.subscription)) ||
        Number(values.subscription) <= 0)
    ) {
      ctx.addIssue({
        code: 'custom',
        path: ['subscription'],
        message: 'Required',
      })
    }
  })
export type RuleFormValues = z.infer<typeof ruleFormSchema>

export function canUseRuleTarget(
  target: AlertTarget,
  values: Pick<RuleFormValues, 'scope' | 'owner' | 'enforced'>,
  managePlatform: boolean
): boolean {
  if (managePlatform) return true
  return !isPlatformScope(values.scope) && target.owner_user_id === values.owner
}

export function ruleDefaults(rule?: AlertRule, owner = 0): RuleFormValues {
  const spec = rule?.config,
    c = spec?.condition
  let threshold = '30000',
    recovery = '10000'
  if (c && isPercentageMetric(c.metric)) {
    threshold = String(c.threshold_bps / 100)
    recovery = String(c.recovery_threshold_bps / 100)
  } else if (c) {
    threshold = String(
      c.threshold_ms ||
        c.threshold_bps ||
        c.threshold_count ||
        c.threshold_usd ||
        0
    )
    recovery = String(
      c.recovery_threshold_ms ||
        c.recovery_threshold_bps ||
        c.recovery_threshold_count ||
        c.recovery_threshold_usd ||
        0
    )
  }
  return {
    quotaUnit: spec?.quota_unit,
    name: spec?.name ?? '',
    baselineMode: c?.baseline?.mode ?? 'historical',
    baselineCount: c?.baseline?.count || 200,
    baselineMinutes: (c?.baseline?.history_seconds || 1800) / 60,
    baselineMinimumCount: c?.baseline?.minimum_count || 20,
    changeDirection: c?.change_direction ?? 'decrease',
    balancePool: c?.balance_pool || 'wallet',
    subscription: c?.subscription_id ? String(c.subscription_id) : '',
    taskType: c?.task_type ?? '',
    legacySignal: c?.legacy_signal,
    scope: spec?.scope.type ?? 'api_key',
    metric: c?.metric ?? 'duration_ms',
    operator: (c?.operator as RuleFormValues['operator']) ?? 'gt',
    threshold,
    recoveryThreshold: recovery,
    recoveryHold: c?.recovery_seconds ?? 30,
    window: c?.window.type || 'rolling',
    minutes: (c?.window.duration_seconds || 300) / 60,
    samples: c?.min_samples ?? 20,
    hold: c?.hold_seconds ?? 30,
    severity: spec?.severity ?? 'P1',
    aggregation: c?.aggregation ?? 'p95',
    deadline: c?.deadline_kind ?? 'first_output',
    deadlineSeconds: (c?.deadline_ms || 30000) / 1000,
    slowSeconds: (c?.slow_threshold_ms || 30000) / 1000,
    trigger: spec?.trigger.mode ?? 'incident',
    targets: (spec?.notify.target_ids ?? []).map(String),
    entities: (spec?.scope.channel_ids?.length
      ? spec.scope.channel_ids
      : (spec?.scope.entity_ids ?? [])
    ).map(String),
    models: spec?.scope.model_selector.models ?? [],
    groupMode: spec?.scope.group_mode ?? 'per_entity',
    allOwned: spec?.scope.selector === 'all_owned',
    allModels: spec?.scope.model_selector.mode === 'all_configured',
    futureModels: spec?.scope.model_selector.include_future_models ?? false,
    reportZero: spec?.trigger.report?.include_zero ?? true,
    reportRevision: spec?.trigger.report?.send_revision ?? true,
    reportFirstPartial: spec?.trigger.report?.allow_first_partial ?? false,
    reportBackfillPeriods: spec?.trigger.report?.backfill_periods ?? 7,
    reportPartialAfterSeconds:
      spec?.trigger.report?.partial_after_seconds ?? 1800,
    notifyRecovery: spec?.notify.on_recovery ?? true,
    includeFuture: spec?.scope.include_future ?? false,
    enabled: spec?.enabled ?? true,
    action: spec?.actions?.[0]?.type ?? '',
    noAlternative: spec?.action_guard?.on_no_alternative ?? 'notify_only',
    minRemainingChannels: spec?.action_guard?.min_remaining_channels ?? 1,
    autoRecovery: spec?.recovery.mode === 'probe',
    sendAt: c?.window.send_at || '00:10',
    timezone: c?.window.timezone || 'Asia/Shanghai',
    stepUSD: spec?.trigger.step_usd ?? '100',
    recoveryDuration: (spec?.recovery.duration_limit_ms ?? 15000) / 1000,
    recoveryProtocol: spec?.recovery.protocol ?? '',
    recoveryTTFT: (spec?.recovery.ttft_limit_ms ?? 5000) / 1000,
    recoverySuccesses: spec?.recovery.successes ?? 3,
    recoveryObserveSeconds: spec?.recovery.observe_seconds ?? 120,
    recoveryModels: spec?.recovery.models ?? [],
    protocol: c?.protocol ?? '',
    stream: c?.stream == null ? '' : String(c.stream),
    owner: rule?.owner_user_id ?? owner,
    enforced: spec?.management_mode === 'platform_enforced',
    repeat: spec?.trigger.repeat_seconds ?? 1800,
    acknowledge: spec?.acknowledgement.required ?? true,
    remindAfterSeconds: spec?.acknowledgement.remind_after_seconds ?? 300,
    escalateAfterSeconds: spec?.acknowledgement.escalate_after_seconds ?? 600,
    escalationTargets: (spec?.acknowledgement.escalation_target_ids ?? []).map(
      String
    ),
  }
}

export function buildRule(values: RuleFormValues): RuleSpec {
  const channel =
      values.scope === 'channel' || values.scope === 'channel_model',
    platform = isPlatformScope(values.scope),
    inflight = values.metric === 'inflight_overdue_count'
  const ids = values.entities.map(Number)
  const probeRecovery = channel && !!values.action && values.autoRecovery
  const condition: RuleSpec['condition'] = {
    balance_pool: values.balancePool,
    subscription_id: Number(values.subscription) || 0,
    task_type: values.taskType,
    legacy_signal: values.legacySignal,
    metric: values.metric,
    baseline:
      values.metric === 'request_volume_change'
        ? {
            mode: values.baselineMode,
            count:
              values.baselineMode === 'manual'
                ? values.baselineCount
                : undefined,
            history_seconds:
              values.baselineMode === 'historical'
                ? values.baselineMinutes * 60
                : undefined,
            minimum_count:
              values.baselineMode === 'historical'
                ? values.baselineMinimumCount
                : undefined,
          }
        : undefined,
    change_direction:
      values.metric === 'request_volume_change'
        ? values.changeDirection
        : undefined,
    aggregation: values.aggregation,
    latency_basis: channel ? 'channel_attempt' : 'gateway_request',
    operator: inflight ? 'gte' : values.operator,
    threshold_ms: Number(values.threshold),
    threshold_count: Number(values.threshold),
    threshold_bps: Number(values.threshold),
    threshold_usd: values.threshold,
    recovery_threshold_ms: Number(values.recoveryThreshold),
    recovery_threshold_count: Number(values.recoveryThreshold),
    recovery_threshold_bps: Number(values.recoveryThreshold),
    recovery_threshold_usd: values.recoveryThreshold,
    slow_threshold_ms: values.slowSeconds * 1000,
    min_samples:
      inflight ||
      [
        'request_count',
        'request_started_count',
        'request_volume_change',
      ].includes(values.metric)
        ? 0
        : values.samples,
    hold_seconds: values.hold,
    recovery_seconds: values.recoveryHold,
    deadline_kind: values.deadline,
    deadline_ms: values.deadlineSeconds * 1000,
    window: inflight
      ? { type: '', duration_seconds: 0, timezone: '', send_at: '' }
      : {
          type: values.window,
          duration_seconds: values.minutes * 60,
          timezone: values.timezone,
          send_at: values.sendAt,
        },
    protocol: values.protocol || undefined,
    stream: values.stream === '' ? undefined : values.stream === 'true',
  }
  const duration = [
      'duration_ms',
      'ttft_ms',
      'source_lag',
      'evaluation_lag',
    ].includes(values.metric),
    rate = isPercentageMetric(values.metric),
    money =
      values.metric === 'spend_net' || values.metric === 'balance_remaining'
  condition.threshold_ms = duration ? Number(values.threshold) : 0
  condition.recovery_threshold_ms = duration
    ? Number(values.recoveryThreshold)
    : 0
  condition.threshold_bps = rate
    ? Math.round(Number(values.threshold) * 100)
    : 0
  condition.recovery_threshold_bps = rate
    ? Math.round(Number(values.recoveryThreshold) * 100)
    : 0
  condition.threshold_count =
    !duration && !rate && !money ? Number(values.threshold) : 0
  condition.recovery_threshold_count =
    !inflight && !duration && !rate && !money
      ? Number(values.recoveryThreshold)
      : 0
  condition.threshold_usd = money ? values.threshold : ''
  condition.recovery_threshold_usd = money ? values.recoveryThreshold : ''
  return {
    name: values.name,
    quota_unit: values.quotaUnit,
    category: values.trigger === 'schedule_report' ? 'report' : 'health',
    severity: values.severity,
    enabled: values.enabled,
    management_mode:
      platform || values.enforced ? 'platform_enforced' : 'user_managed',
    scope: {
      type: values.scope,
      selector: values.allOwned ? 'all_owned' : 'ids',
      entity_ids: channel ? [] : ids,
      channel_ids: channel ? ids : [],
      group_mode: values.groupMode,
      include_future: values.includeFuture,
      model_selector: {
        mode:
          values.scope !== 'model' && values.allModels
            ? 'all_configured'
            : 'explicit',
        models: values.models,
        include_future_models: values.scope !== 'model' && values.futureModels,
      },
    },
    condition,
    trigger: {
      mode: inflight ? 'incident' : values.trigger,
      repeat_seconds: values.repeat,
      step_usd: values.stepUSD,
      report: {
        include_zero: values.reportZero,
        send_revision: values.reportRevision,
        allow_first_partial: values.reportFirstPartial,
        backfill_periods: values.reportBackfillPeriods,
        partial_after_seconds: values.reportPartialAfterSeconds,
      },
    },
    notify: {
      target_ids: values.targets.map(Number),
      on_recovery: values.notifyRecovery,
    },
    actions:
      values.action && !inflight
        ? [{ type: values.action, target: 'matched_entity' }]
        : [],
    action_guard: channel
      ? {
          min_remaining_channels: values.minRemainingChannels,
          on_no_alternative: values.noAlternative,
        }
      : undefined,
    recovery: {
      protocol: values.recoveryProtocol,
      mode: probeRecovery ? 'probe' : 'manual',
      duration_limit_ms: values.recoveryDuration * 1000,
      ttft_limit_ms: values.recoveryTTFT * 1000,
      successes: probeRecovery
        ? Math.max(3, values.recoverySuccesses)
        : values.recoverySuccesses,
      observe_seconds: probeRecovery
        ? Math.max(120, values.recoveryObserveSeconds)
        : values.recoveryObserveSeconds,
      models: values.recoveryModels,
    },
    acknowledgement: {
      required: values.acknowledge,
      remind_after_seconds: values.remindAfterSeconds,
      escalate_after_seconds: values.escalateAfterSeconds,
      escalation_target_ids: values.escalationTargets.map(Number),
    },
  }
}

export function metricAllowed(metric: string, scope: AlertScope): boolean {
  const channel = scope === 'channel' || scope === 'channel_model'
  if (scope === 'model') {
    return [
      'request_count',
      'request_started_count',
      'request_volume_change',
    ].includes(metric)
  }
  if (scope === 'uid_budget' || scope === 'project_budget') {
    return metric === 'budget_remaining_ratio'
  }
  if (metric === 'budget_remaining_ratio') {
    return false
  }
  if (metric === 'balance_remaining') {
    return scope === 'account'
  }
  if (
    [
      'task_failure_count',
      'task_overdue',
      'evaluation_lag',
      'source_lag',
      'delivery_backlog',
    ].includes(metric)
  ) {
    return scope === 'system'
  }
  if (metric === 'inflight_overdue_count') return channel || scope === 'system'
  if (metric === 'consecutive_failures') return channel
  if (
    [
      'spend_net',
      'input_tokens',
      'output_tokens',
      'cached_tokens',
      'cache_write_5m_tokens',
      'cache_write_1h_tokens',
    ].includes(metric)
  ) {
    return !channel
  }
  return true
}

export function metricDefaults(
  metric: string
): Pick<
  RuleFormValues,
  'metric' | 'threshold' | 'recoveryThreshold' | 'operator'
> {
  let threshold = '1000',
    recoveryThreshold = '500',
    operator: RuleFormValues['operator'] = 'gt'
  if (
    ['duration_ms', 'ttft_ms', 'source_lag', 'evaluation_lag'].includes(metric)
  ) {
    threshold = '30000'
    recoveryThreshold = '10000'
  } else if (metric === 'request_volume_change') {
    threshold = '50'
    recoveryThreshold = '20'
    operator = 'gte'
  } else if (metric === 'inflight_overdue_count') {
    threshold = '1'
    recoveryThreshold = '0'
  } else if (metric === 'spend_net') {
    threshold = '100'
    recoveryThreshold = '0'
  } else if (metric === 'success_rate') {
    threshold = '95'
    recoveryThreshold = '99'
    operator = 'lt'
  } else if (metric.endsWith('_rate')) {
    threshold = '5'
    recoveryThreshold = '2'
  } else if (metric === 'budget_remaining_ratio') {
    threshold = '20'
    recoveryThreshold = '30'
    operator = 'lt'
  } else if (metric === 'balance_remaining') {
    threshold = '10'
    recoveryThreshold = '20'
    operator = 'lt'
  }
  return { metric, threshold, recoveryThreshold, operator }
}

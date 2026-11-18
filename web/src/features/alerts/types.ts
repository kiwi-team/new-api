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
import type { VerificationOperation } from '@/features/auth/secure-verification/types'

export type AlertOperation = Extract<
  VerificationOperation,
  { scope: 'alert.manage' }
>
export type AlertScope =
  | 'account'
  | 'api_key'
  | 'channel'
  | 'channel_model'
  | 'model'
  | 'system'
  | 'uid_budget'
  | 'project_budget'
export type RuleSpec = {
  quota_unit?: number
  name: string
  category: string
  severity: string
  enabled: boolean
  management_mode: string
  scope: {
    type: AlertScope
    selector: string
    entity_ids?: number[] | null
    channel_ids?: number[] | null
    group_mode: string
    include_future: boolean
    model_selector: {
      mode: string
      models: string[] | null
      include_future_models: boolean
    }
  }
  condition: {
    baseline?: {
      mode: 'manual' | 'historical'
      count?: number
      history_seconds?: number
      minimum_count?: number
    }
    change_direction?: 'increase' | 'decrease'
    legacy_signal?: string
    balance_pool?: string
    subscription_id?: number
    task_type?: string
    metric: string
    aggregation: string
    latency_basis: string
    operator: string
    threshold_ms: number
    threshold_count: number
    threshold_bps: number
    threshold_usd: string
    recovery_threshold_ms: number
    recovery_threshold_count: number
    recovery_threshold_bps: number
    recovery_threshold_usd: string
    slow_threshold_ms: number
    min_samples: number
    hold_seconds: number
    recovery_seconds: number
    deadline_kind: string
    deadline_ms: number
    window: {
      type: string
      duration_seconds: number
      timezone: string
      send_at: string
    }
    protocol?: string
    stream?: boolean | null
  }
  trigger: {
    mode: string
    repeat_seconds: number
    step_usd: string
    report?: {
      include_zero?: boolean
      allow_first_partial: boolean
      send_revision: boolean
      backfill_periods: number
      partial_after_seconds: number
    }
  }
  notify: { target_ids: number[] | null; on_recovery: boolean }
  actions: { type: string; target: string }[] | null
  action_guard?: { min_remaining_channels: number; on_no_alternative: string }
  recovery: {
    protocol?: string
    mode: string
    ttft_limit_ms: number
    duration_limit_ms: number
    successes: number
    observe_seconds: number
    models?: string[] | null
  }
  acknowledgement: {
    required: boolean
    remind_after_seconds: number
    escalate_after_seconds: number
    escalation_target_ids: number[] | null
  }
}
export type AlertRule = {
  config_valid?: boolean
  id: number
  owner_user_id: number
  revision: number
  effective_at: number
  last_evaluation_error?: string
  config: RuleSpec
  states?: RuleState[]
  targets?: { type: string; id: number; name: string; model: string }[]
  runtime?: { data_status: string; target_count: number; updated_at: number }
}
export type RuleState = {
  target_name: string
  id: string
  status: string
  data_status: string
  last_value: string
  last_count: string
  model_key: string
}
export type AlertTarget = {
  id: number
  owner_user_id: number
  name: string
  type: string
  enabled: boolean
  language: string
  revision: number
  address_hint: string
  last_test_status: string
}
export type TargetInput = Pick<
  AlertTarget,
  'name' | 'type' | 'enabled' | 'language'
> & {
  expected_revision: number
  credentials?: {
    private_network?: boolean
    address: string
    secret: string
    token: string
    priority: number
  }
}
export type AlertEvent = {
  id: number
  rule_id: number
  incident_id: number
  kind: string
  severity: string
  target_name: string
  summary: string
  evidence: string
  created_at: number
  condition_satisfied_at: number
}
export type AlertIncident = {
  owner_user_id: number
  close_reason: string
  id: number
  rule_id: number
  severity: string
  target_name: string
  status: string
  handling_status: string
  opened_at: number
}
export type AlertJob = {
  target_name: string
  target_type: string
  target_hint: string
  id: number
  kind: string
  action: string
  status: string
  last_error: string
  attempts: number
  accepted_at: number
  target_id: number
}
export type AlertHold = {
  id: number
  rule_id: number
  owner_user_id: number
  target_type: string
  target_name: string
  channel_id: number
  token_id: number
  model_key: string
  active: boolean
  platform_enforced: boolean
  last_probe_status: string
  probe_count: number
}
export type HoldResponse = {
  hold: AlertHold
  verification_context: AlertOperation['context']
}
export type ChannelOption = {
  id: number
  name: string
  models: string
  status: number
  protection_mode?: string
  protection_revision?: number
}
export type NamedOption = { id: number; name: string }
export type Catalog = {
  shadow_mode?: boolean
  scopes: AlertScope[]
  metrics: string[]
  target_types: string[]
  enabled: boolean
  actions_enabled: boolean
  encryption_configured: boolean
  read_all: boolean
  manage_platform: boolean
  read_engine_health: boolean
  api_keys: NamedOption[]
  subscriptions?: NamedOption[]
  budget_options?: (NamedOption & { type: AlertScope })[]
}
export type Measurement = {
  traffic?: {
    count: string
    baseline_scaled: string
    change_bps: string
    baseline_start: number
    baseline_end: number
    current_complete: boolean
    frozen: boolean
  }
  value: string
  samples: string
  numerator: string
  missing: string
  status: string
  reason?: string
  window_start: number
  window_end: number
  request_ids: string[]
}
export type Preview = {
  config: RuleSpec
  verification_context: AlertOperation['context']
  targets: {
    target: { type: string; id: number; name: string; model: string }
    measurement: Measurement
    impact?: ActionImpact
  }[]
}
export type ActionImpact = {
  status: string
  reason?: string
  domains: { group: string; model: string; alternatives: string[] }[]
}
export type EventDetail = {
  can_manage: boolean
  event: AlertEvent
  jobs: AlertJob[]
  config: RuleSpec
}
export type EngineHealth = {
  enabled: boolean
  actions_enabled: boolean
  encryption_configured: boolean
  failed_rules: number
  receive_mode: string
  uncovered_nodes: string[]
  job_backlog: string
  sources: {
    id: string
    node_id: string
    kind: string
    protocol_version: number
    usage_healthy: boolean
    updated_at: number
    healthy: boolean
    offered_sequence: number
    accepted_count: number
    accepted_through: number
    last_error: string
    captured_at: number
  }[]
}

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
import type { TFunction } from 'i18next'

// Stored enums are stable protocol values; presentation uses English source
// keys so all seven interface languages can translate them consistently.
export function alertLabel(value: string, t: TFunction): string {
  switch (value) {
    case 'token_spend':
      return t('API Keys')
    case 'user_spend':
      return t('Account')
    case 'user_report':
    case 'schedule_report':
      return t('Usage report')
    case 'legacy_notice':
    case 'legacy_event_trigger':
      return t('Legacy trigger retained')
    case 'uid_day':
      return t('UID daily spending')
    case 'uid_budget_notice':
      return t('UID and project budgets')
    case 'channel_arrears':
      return t('Channel balance warning')
    case 'slow_error':
      return t('Slow channel error')
    case 'user_notifications':
      return t('Account and channel notifications')
    case 'key_hour':
      return t('Natural hour')
    case 'key_day':
      return t('Natural day')
    case 'actions_disabled':
      return t('Automatic actions disabled')
    case 'shadow_mode':
      return t('Shadow mode')
    case 'target_changed':
      return t('Notification target changed')
    case 'target_unavailable':
    case 'delivery_unavailable':
      return t('Notification target unavailable')
    case 'target_rejected':
    case 'delivery_failed':
      return t('Delivery failed')
    case 'condition_changed':
      return t('Condition no longer met')
    case 'quota_unit_changed':
      return t('Quota unit changed')
    case 'registry_unavailable':
    case 'checkpoint_unavailable':
      return t('Source unavailable')
    case 'data_incomplete':
      return t('Usage input incomplete')
    case 'rate_limited':
      return t('Rate limited')
    case 'source_capacity':
    case 'lifecycle_capacity':
    case 'report_key_capacity':
    case 'failure_sequence_capacity':
      return t('Capacity limit reached')
    case 'evaluator_starting':
      return t('Warming up')
    case 'invalid_configuration':
    case 'configuration_invalid':
      return t('Invalid configuration')
    case 'authorization_revoked':
      return t('Authorization revoked')
    case 'rule_paused':
      return t('Rule paused')
    case 'rule_changed':
      return t('Rule changed')
    case 'rule_unavailable':
      return t('Rule unavailable')
    case 'channel_changed':
      return t('Channel configuration changed')
    case 'usage_input_incomplete':
    case 'usage_input_backlog':
    case 'usage_input_unavailable':
    case 'usage_source_disabled':
      return t('Usage input incomplete')
    case 'producer_window_coverage':
    case 'historical_source_gap':
      return t('Waiting for a complete window')
    case 'protocol_correlation_unsupported':
      return t('Output observation is unsupported')
    case 'probe_unavailable':
      return t('Probe unavailable')
    case 'sql_first':
      return t('Database input')
    case 'sql_fallback':
      return t('Database fallback')
    case 'redis_stream':
      return t('Redis input')
    case 'engine_health':
      return t('Engine status')
    case 'report':
      return t('Usage report')
    case 'report_revision':
      return t('Revised usage report')
    case 'report_data_gap':
      return t('Report data coverage incomplete')
    case 'attachment_routing_requires_review':
      return t('Attachment routing needs review')
    case 'manual':
      return t('Manual')
    case 'migrations':
      return t('Legacy migrations')
    case 'observing':
      return t('Observing')
    case 'ambiguous':
      return t('Baseline needs review')
    case 'uid_budget':
      return t('UID budget')
    case 'project_budget':
      return t('Project budget')
    case 'budget_remaining_ratio':
      return t('Remaining budget ratio')
    case 'balance_remaining':
      return t('Remaining balance')
    case 'spend_net':
      return t('Net spending')
    case 'request_count':
      return t('Request count')
    case 'input_tokens':
      return t('Input tokens')
    case 'output_tokens':
      return t('Output tokens')
    case 'cached_tokens':
      return t('Cache read tokens')
    case 'cache_write_5m_tokens':
      return t('5-minute cache write tokens')
    case 'cache_write_1h_tokens':
      return t('1-hour cache write tokens')
    case 'success_rate':
      return t('Success rate')
    case 'failure_rate':
      return t('Failure rate')
    case 'timeout_rate':
      return t('Timeout rate')
    case 'timeout_count':
      return t('Timeout count')
    case 'consecutive_failures':
      return t('Consecutive failures')
    case 'ttft_ms':
      return t('First output latency')
    case 'duration_ms':
      return t('Request duration')
    case 'slow_request_count':
      return t('Slow request count')
    case 'slow_request_rate':
      return t('Slow request rate')
    case 'inflight_overdue_count':
      return t('Overdue operations in flight')
    case 'task_failure_count':
      return t('Failed background tasks')
    case 'task_overdue':
      return t('Overdue background tasks')
    case 'evaluation_lag':
      return t('Evaluation lag')
    case 'source_lag':
      return t('Source lag')
    case 'delivery_backlog':
      return t('Delivery backlog')
    case 'account':
      return t('Account')
    case 'api_key':
    case 'api_key_set':
      return t('API Keys')
    case 'channel':
      return t('Channel')
    case 'channel_model':
      return t('Channel model')
    case 'system':
      return t('System')
    case 'rules':
      return t('Rules')
    case 'incidents':
      return t('Current incidents')
    case 'events':
      return t('Event history')
    case 'targets':
      return t('Notification targets')
    case 'holds':
      return t('Isolation sources')
    case 'engine':
      return t('Engine status')
    case 'channels':
      return t('Channel protection')
    case 'rolling':
      return t('Rolling window')
    case 'hour':
      return t('Calendar hour')
    case 'day':
      return t('Calendar day')
    case 'month':
      return t('Calendar month')
    case 'cumulative':
      return t('Since activation')
    case 'first_output':
      return t('No first output')
    case 'output_idle':
      return t('Output stalled')
    case 'total':
      return t('Total duration')
    case 'mean':
      return t('Mean')
    case 'complete':
      return t('Complete')
    case 'partial':
      return t('Partial')
    case 'stale':
      return t('Stale data')
    case 'warming_up':
      return t('Warming up')
    case 'no_data':
      return t('No data')
    case 'unknown':
      return t('Unknown')
    case 'pending':
      return t('Pending')
    case 'firing':
      return t('Firing')
    case 'resolved':
      return t('Resolved')
    case 'unacknowledged':
      return t('Unacknowledged')
    case 'acknowledged':
      return t('Acknowledged')
    case 'closed':
      return t('Closed')
    case 'running':
      return t('Running')
    case 'succeeded':
      return t('Succeeded')
    case 'failed':
      return t('Failed')
    case 'blocked':
      return t('Blocked')
    case 'canceled':
      return t('Canceled')
    case 'retry_wait':
      return t('Waiting to retry')
    case 'expired':
      return t('Expired')
    case 'shadow':
      return t('Shadow mode')
    case 'user_managed':
      return t('User managed')
    case 'platform_enforced':
      return t('Platform enforced')
    case 'legacy':
      return t('Legacy protection')
    case 'scoped':
      return t('Scoped protection')
    case 'notify':
      return t('Notification')
    case 'action':
      return t('Automatic action')
    case 'disable_api_key':
      return t('Disable matched API keys')
    case 'disable_account_keys':
      return t('Disable all account API keys')
    case 'disable_channel_model':
      return t('Isolate matched channel model')
    case 'disable_channel':
      return t('Disable whole channel')
    case 'qualified':
      return t('Probe qualified')
    case 'not_qualified':
      return t('Probe not qualified')
    case 'no_alternative':
      return t('No alternative channels')
    case 'legacy_protection_conflict':
      return t('Switch to scoped protection first')
    case 'automatic_protection_disabled':
      return t('Channel automatic protection is disabled')
    case 'restricted_routing_requires_review':
      return t('Restricted routing needs review')
    case 'producer_not_covered':
    case 'producer_registry_empty':
      return t('No covered producers')
    case 'producer_unavailable':
    case 'source_unavailable':
      return t('Source unavailable')
    case 'input_backlog':
      return t('Input backlog')
    case 'output_unsupported':
      return t('Output observation is unsupported')
    case 'insufficient_samples':
      return t('Insufficient samples')
    case 'window_coverage':
    case 'first_complete_period':
      return t('Waiting for a complete window')
    case 'slow':
      return t('Slow completed requests')
    case 'normal':
      return t('Normal')
    case 'inflight':
      return t('Requests without a terminal observation')
    case 'clock_changed':
      return t('Clock changed; correct the time and restart this collector')
    case 'clock_skew':
      return t('Clock skew detected')
    case 'evaluation_budget':
      return t('Evaluation will resume with the remaining objects')
    case 'evaluation_failed':
      return t('Alert operation failed')
    default:
      return t(value)
  }
}

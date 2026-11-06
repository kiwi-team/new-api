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
import type { RuleFormValues } from './rule-form'

// Templates contain defaults only. Users select their own channels, models and
// destinations; automatic actions always require a separate explicit choice.
export const ruleTemplates: {
  name: string
  platform: boolean
  values: Partial<RuleFormValues>
}[] = [
  {
    name: 'Channel model latency',
    platform: true,
    values: {
      enabled: false,
      scope: 'channel_model',
      metric: 'duration_ms',
      aggregation: 'p95',
      threshold: '30000',
      recoveryThreshold: '10000',
      samples: 20,
      hold: 30,
      enforced: true,
    },
  },
  {
    name: 'Channel model failures',
    platform: true,
    values: {
      enabled: false,
      scope: 'channel_model',
      metric: 'failure_rate',
      threshold: '5000',
      recoveryThreshold: '1000',
      samples: 20,
      hold: 30,
      enforced: true,
    },
  },
  {
    name: 'Waiting for first output',
    platform: true,
    values: {
      enabled: false,
      scope: 'channel_model',
      metric: 'inflight_overdue_count',
      threshold: '1',
      recoveryThreshold: '0',
      operator: 'gte',
      deadline: 'first_output',
      deadlineSeconds: 30,
      samples: 0,
      hold: 0,
      enforced: true,
    },
  },
  {
    name: 'Output stalled',
    platform: true,
    values: {
      enabled: false,
      scope: 'channel_model',
      metric: 'inflight_overdue_count',
      threshold: '1',
      recoveryThreshold: '0',
      operator: 'gte',
      deadline: 'output_idle',
      deadlineSeconds: 30,
      samples: 0,
      hold: 0,
      enforced: true,
    },
  },
  {
    name: 'API key spending milestones',
    platform: false,
    values: {
      enabled: false,
      scope: 'api_key',
      metric: 'spend_net',
      threshold: '100',
      recoveryThreshold: '0',
      operator: 'gte',
      window: 'cumulative',
      trigger: 'milestone',
      stepUSD: '100',
      samples: 1,
      hold: 0,
      severity: 'P2',
    },
  },
  {
    name: 'Daily account summary',
    platform: false,
    values: {
      enabled: false,
      scope: 'account',
      metric: 'spend_net',
      threshold: '0',
      recoveryThreshold: '0',
      operator: 'gte',
      window: 'day',
      trigger: 'schedule_report',
      samples: 0,
      hold: 0,
      severity: 'P3',
      groupMode: 'aggregate',
    },
  },
]

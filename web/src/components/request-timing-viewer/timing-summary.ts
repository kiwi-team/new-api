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

export type TimingRecord = Record<string, unknown>
export type TimingRow = {
  id?: string
  label: string
  value?: number
  reason?: string
  note?: string
}
export type TimingSection = {
  title: string
  description?: string
  rows: TimingRow[]
}

export function timingRecord(value: unknown): TimingRecord | undefined {
  if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
    return value as TimingRecord
  }
  return undefined
}

function timingRecords(value: unknown): TimingRecord[] {
  if (!Array.isArray(value)) {
    return []
  }
  return value
    .map(timingRecord)
    .filter((entry): entry is TimingRecord => entry !== undefined)
}

function milliseconds(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? value
    : undefined
}

function interval(
  record: TimingRecord,
  start: string,
  end: string
): number | undefined {
  const first = milliseconds(record[start])
  const last = milliseconds(record[end])
  if (first === undefined || last === undefined || last < first) {
    return undefined
  }
  return last - first
}

function duration(
  label: string,
  value: unknown,
  reason = 'not_observed',
  note?: string
): TimingRow {
  return { label, value: milliseconds(value), reason, note }
}

function intervalReason(
  record: TimingRecord,
  start: string,
  end: string
): string {
  return milliseconds(record[start]) !== undefined && record[end] == null
    ? 'pending_at_finalize'
    : 'not_observed'
}

// Only pair recorded boundaries. Never subtract a snapshot time to manufacture
// a final duration, or add overlapping streaming intervals into a request total.
export function buildTimingSummary(
  snapshot: TimingRecord,
  t: TFunction
): TimingSection[] {
  const sections: TimingSection[] = []
  const request = timingRecord(snapshot.request)
  const isBackground = snapshot.scope === 'background_operation'
  const coverage = timingRecord(snapshot.coverage)
  const missing = timingRecord(snapshot.missing_reasons)
  const separatedParsing =
    typeof snapshot.schema_version === 'number' && snapshot.schema_version >= 2
  const parsingLabel = separatedParsing
    ? t('Business request parsing and validation (overall)')
    : t('Request parsing (legacy, unspecified stage)')
  if (request) {
    const offsets = timingRecord(request.offsets_ms) ?? {}
    const totalReason =
      missing?.['request.handler_ended_at_ms'] === 'pending_at_finalize' ||
      ((snapshot.phase === 'consume_log' || snapshot.phase === 'error_log') &&
        request.gateway_total_ms == null)
        ? 'pending_at_finalize'
        : 'not_observed'
    const requestRows = [
      duration(
        isBackground ? t('Operation duration') : t('Request total'),
        request.gateway_total_ms,
        totalReason
      ),
      duration(t('Before the first upstream request'), request.pre_upstream_ms),
      duration(
        t('Before reading the request body'),
        request.before_body_read_ms,
        request.body_state === 'no_body' ? 'not_applicable' : 'not_observed'
      ),
      duration(
        t('Receiving the request body'),
        request.body_read_ms,
        request.body_state === 'no_body'
          ? 'not_applicable'
          : intervalReason(
              offsets,
              'body_read_started_at_ms',
              'body_read_ended_at_ms'
            ),
        request.body_state === 'interrupted' ? t('Interrupted') : undefined
      ),
      duration(
        t('Authentication'),
        request.auth_ms,
        intervalReason(offsets, 'auth_started_at_ms', 'auth_ended_at_ms')
      ),
      duration(
        t('Model extraction'),
        request.model_extract_ms,
        intervalReason(
          offsets,
          'model_extract_started_at_ms',
          'model_extract_ended_at_ms'
        )
      ),
      duration(
        t('Channel routing'),
        request.route_ms,
        intervalReason(offsets, 'route_started_at_ms', 'route_ended_at_ms')
      ),
      duration(
        parsingLabel,
        request.request_parse_ms,
        intervalReason(
          offsets,
          'request_parse_started_at_ms',
          'request_parse_ended_at_ms'
        )
      ),
      duration(
        t('Body ready to upstream request'),
        request.after_body_ready_to_upstream_ms
      ),
      duration(
        t('Response cleanup'),
        request.post_response_ms,
        intervalReason(
          offsets,
          'response_handling_ended_at_ms',
          'handler_ended_at_ms'
        )
      ),
    ]
    sections.push({
      title: isBackground ? t('Background operation') : t('Request stages'),
      rows: isBackground ? requestRows.slice(0, 2) : requestRows,
    })
  }

  const phaseLabels: Record<string, string> = {
    auth: t('Authentication'),
    model_extract: t('Model extraction'),
    route: t('Channel routing'),
    auth_tag_decode: t('Authentication tag decoding'),
    request_parse: parsingLabel,
    request_decode: t('Request body decoding (substep)'),
    request_prepare: t('Request preparation'),
    task_wait: t('Waiting for task completion'),
    plugin_build_request: t('Plugin request preparation'),
    plugin_render: t('Plugin response rendering'),
  }
  const spans = timingRecords(snapshot.business_spans)
  if (spans.length) {
    let parsingDescription: string | undefined
    if (
      !separatedParsing &&
      spans.some((span) => span.phase === 'request_parse')
    ) {
      parsingDescription = t(
        'Legacy parsing timings do not distinguish authentication decoding, business validation, or body decoding.'
      )
    } else if (
      spans.some(
        (span) =>
          span.phase === 'auth_tag_decode' || span.phase === 'request_decode'
      )
    ) {
      parsingDescription = t(
        'Decoding substeps can be included in enclosing stages; do not add their durations.'
      )
    }
    sections.push({
      title: t('Processing intervals'),
      description: parsingDescription,
      rows: spans.map((span, index) => ({
        ...duration(
          typeof span.phase === 'string'
            ? (phaseLabels[span.phase] ?? span.phase)
            : t('Processing'),
          span.duration_ms,
          span.status === 'pending_at_finalize' || span.status === 'in_progress'
            ? 'pending_at_finalize'
            : 'not_observed'
        ),
        id:
          typeof span.span_id === 'string'
            ? span.span_id
            : `${String(span.phase)}-${index}`,
      })),
    })
  }

  for (const [index, attempt] of timingRecords(snapshot.attempts).entries()) {
    const id =
      typeof attempt.attempt_id === 'string'
        ? attempt.attempt_id
        : String(index + 1)
    const channel =
      typeof attempt.channel_id === 'number'
        ? ` · ${t('Channel')} #${attempt.channel_id}`
        : ''
    sections.push({
      title: t('Channel attempt {{id}}', { id }) + channel,
      description: t('Event times are offsets from gateway entry.'),
      rows: [
        duration(
          t('Attempt duration'),
          interval(attempt, 'attempt_started_at_ms', 'attempt_ended_at_ms'),
          intervalReason(
            attempt,
            'attempt_started_at_ms',
            'attempt_ended_at_ms'
          )
        ),
        duration(t('First protocol event'), attempt.first_protocol_event_at_ms),
        duration(t('First SSE data'), attempt.first_sse_data_at_ms),
        duration(t('First output'), attempt.first_output_at_ms),
        duration(t('Protocol terminal event'), attempt.protocol_terminal_at_ms),
      ],
    })
  }

  for (const [index, call] of timingRecords(
    snapshot.outbound_calls
  ).entries()) {
    const id =
      typeof call.call_id === 'string' ? call.call_id : String(index + 1)
    const details = [call.purpose, call.attempt_id, call.transport_kind]
      .filter((value): value is string => typeof value === 'string')
      .join(' · ')
    let reason = 'not_observed'
    if (call.transport_kind === 'sdk_opaque') reason = 'unsupported_transport'
    let writeReason = reason
    if (call.early_response === true) writeReason = 'early_response'
    else if (call.missing_reason === 'ambiguous_transport_retry') {
      writeReason = 'ambiguous_transport_retry'
    }
    const connectionReason =
      call.connection_reused === true ? 'not_applicable' : reason
    sections.push({
      title: t('Outbound call {{id}}', { id }),
      description: details,
      rows: [
        duration(
          t('Call duration'),
          call.call_total_ms,
          call.status === 'pending_at_finalize' ? 'pending_at_finalize' : reason
        ),
        duration(t('Connection wait'), call.connection_wait_ms, reason),
        duration(
          'DNS',
          interval(call, 'dns_started_at_ms', 'dns_ended_at_ms'),
          connectionReason
        ),
        duration(
          'TCP',
          interval(call, 'connect_started_at_ms', 'connect_ended_at_ms'),
          connectionReason
        ),
        duration(
          'TLS',
          interval(call, 'tls_started_at_ms', 'tls_ended_at_ms'),
          connectionReason
        ),
        duration(
          t('Sending the upstream request'),
          call.upstream_body_write_ms,
          writeReason
        ),
        duration(
          t('Waiting for upstream response headers'),
          call.upstream_wait_headers_ms,
          writeReason
        ),
        duration(
          t('Receiving upstream data (span)'),
          call.upstream_receive_span_ms,
          reason
        ),
        duration(
          t('Upstream reads (sum)'),
          call.upstream_read_call_sum_ms,
          reason
        ),
        duration(
          t('Longest upstream read'),
          call.upstream_read_call_max_ms,
          reason
        ),
        duration(t('Upstream tail wait'), call.upstream_tail_wait_ms, reason),
      ],
    })
  }

  const downstream = timingRecord(snapshot.downstream)
  if (downstream && coverage?.downstream !== 'not_applicable') {
    sections.push({
      title: t('Downstream delivery'),
      rows: [
        duration(t('Downstream write span'), downstream.write_span_ms),
        duration(t('Downstream writes (sum)'), downstream.write_call_sum_ms),
        duration(t('Longest downstream write'), downstream.write_call_max_ms),
        duration(t('Flush calls (sum)'), downstream.flush_call_sum_ms),
        duration(t('Longest flush call'), downstream.flush_call_max_ms),
      ],
    })
  }
  return sections
}

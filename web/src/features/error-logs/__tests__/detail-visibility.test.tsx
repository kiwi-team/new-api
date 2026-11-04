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
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'

import { ErrorLogDetailSheet } from '../components/error-log-detail-sheet'
import type { ErrorLog } from '../types'

const log: ErrorLog = {
  id: 1,
  user_id: 1,
  created_at: 1_793_750_400,
  channel_id: 0,
  channel_name: '',
  token_id: 0,
  token_name: '',
  model_name: 'ip-model',
  message: 'Upstream failed',
  type: 'upstream_error',
  param: '',
  code: 'bad_gateway',
  request_id: 'ip-error-request',
  upstream_request_id: 'provider-upstream-id',
  status_code: 502,
  use_time_ms: 100,
  ip: '203.0.113.8',
  client_user_id: '',
  client_scenairo: '',
  session_id: '',
}
let client: QueryClient

afterEach(() => {
  cleanup()
  client.clear()
})

function renderDetails(isRoot: boolean, entry: ErrorLog = log): void {
  client = new QueryClient()
  client.setQueryData(['error-logs', 'body', log.id], {
    success: true,
    data: '',
  })
  client.setQueryData(['error-logs', 'header', log.id], {
    success: true,
    data: { content: '' },
  })
  client.setQueryData(['error-logs', 'timing', log.id], {
    success: true,
    data: { content: '' },
  })
  client.setDefaultOptions({ queries: { staleTime: Infinity, retry: false } })
  render(
    <QueryClientProvider client={client}>
      <ErrorLogDetailSheet
        log={entry}
        canViewHeader={isRoot}
        onOpenChange={() => undefined}
      />
    </QueryClientProvider>
  )
}

test.each([false, true])(
  'error details expose the recorded IP only when root access is %s',
  (isRoot) => {
    renderDetails(isRoot)
    if (isRoot) {
      expect(screen.getByText('203.0.113.8')).toBeVisible()
      expect(screen.getByText('IP')).toBeVisible()
    } else {
      expect(screen.queryByText('203.0.113.8')).not.toBeInTheDocument()
      expect(screen.queryByText('IP')).not.toBeInTheDocument()
    }
  }
)

test('root error details show the upstream request ID separately from the gateway request ID', () => {
  renderDetails(true)

  expect(screen.getByText('Upstream Request ID')).toBeVisible()
  expect(screen.getByText('provider-upstream-id')).toBeVisible()
  expect(screen.getByText('ip-error-request')).toBeVisible()
})

test.each([undefined, ''])(
  'error details omit the upstream request ID when the upstream did not supply one (%s)',
  (upstreamRequestID) => {
    renderDetails(true, { ...log, upstream_request_id: upstreamRequestID })

    expect(screen.queryByText('Upstream Request ID')).not.toBeInTheDocument()
    expect(screen.getByText('ip-error-request')).toBeVisible()
  }
)

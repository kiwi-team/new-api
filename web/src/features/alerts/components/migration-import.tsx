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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'

import { alertRequest } from '../api'
import { alertLabel } from '../lib/labels'

export type MigrationCandidate = {
  kind: string
  object_id: string
  name: string
  owner_user_id: number
  threshold_usd: string
  configured: boolean
}

type ImportResult = {
  kind: string
  object_id: string
  success: boolean
  error?: { message_key: string }
}

export function MigrationImport(props: {
  candidates: MigrationCandidate[]
  disabled: boolean
  onPendingChange: (pending: boolean) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [open, setOpen] = useState(false)
  const [results, setResults] = useState<(ImportResult & { name: string })[]>(
    []
  )
  const configured = props.candidates.filter((item) => item.configured)
  const imported = useMutation({
    mutationFn: async () => {
      props.onPendingChange(true)
      setResults([])
      // Small ordered batches avoid parallel imports competing for the same
      // legacy checkpoint lease. Completed batches remain visible on failure.
      for (let start = 0; start < configured.length; start += 10) {
        const batch = configured.slice(start, start + 10)
        const response = await alertRequest<{ items: ImportResult[] }>(
          'migrations/batch',
          'POST',
          {
            items: batch.map((item) => ({
              kind: item.kind,
              object_id: item.object_id,
            })),
          }
        )
        setResults((previous) => [
          ...previous,
          ...response.items.map((item, index) => ({
            ...item,
            name:
              batch[index].owner_user_id === 0 ? item.kind : batch[index].name,
          })),
        ])
      }
    },
    onSuccess: () => setOpen(false),
    onSettled: () => {
      props.onPendingChange(false)
      void client.invalidateQueries({ queryKey: ['alerts'] })
    },
  })
  const succeeded = results.filter((item) => item.success).length
  return (
    <div className='space-y-3'>
      <Button
        variant='outline'
        disabled={
          props.disabled || imported.isPending || configured.length === 0
        }
        onClick={() => setOpen(true)}
      >
        {t('Import configured alerts ({{count}})', {
          count: configured.length,
        })}
      </Button>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Only configured destinations in this list are included. Existing imports are reused; failed items can be retried.'
        )}
      </p>
      {results.length > 0 && (
        <div role='status'>
          {t('Import results: {{succeeded}} succeeded, {{failed}} failed.', {
            succeeded,
            failed: results.length - succeeded,
          })}
        </div>
      )}
      {results.some((item) => !item.success) && (
        <StaticDataTable
          data={results.filter((item) => !item.success)}
          getRowKey={(item) => `${item.kind}:${item.object_id}`}
          columns={[
            {
              id: 'name',
              header: t('Monitored object'),
              cell: (item) => (
                <div>
                  {item.object_id === 'site'
                    ? alertLabel(item.kind, t)
                    : item.name}
                  <p className='text-muted-foreground text-xs'>
                    {alertLabel(item.kind, t)}
                  </p>
                </div>
              ),
            },
            {
              id: 'error',
              header: t('Failure reason'),
              cell: (item) =>
                t(item.error?.message_key || 'Alert operation failed'),
            },
          ]}
        />
      )}
      <ConfirmDialog
        open={open}
        onOpenChange={(value) => {
          if (!imported.isPending) setOpen(value)
        }}
        title={t('Import configured alerts ({{count}})', {
          count: configured.length,
        })}
        desc={t(
          'Copy the listed alert rules, destinations and checkpoints into the alert center as observers. Legacy alerts continue sending until each migration is cut over.'
        )}
        confirmText={t('Import as observer')}
        disabled={props.disabled || configured.length === 0}
        isLoading={imported.isPending}
        handleConfirm={() => imported.mutate()}
      />
    </div>
  )
}

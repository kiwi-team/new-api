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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { StaticDataTable } from '@/components/data-table'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'

import { alertRequest } from '../api'
import { alertLabel } from '../lib/labels'
import type { AlertOperation, Catalog } from '../types'
import { MigrationImport, type MigrationCandidate } from './migration-import'

type Migration = {
  id: string
  kind: string
  object_id: string
  rule_id: number
  owner_user_id: number
  executor: string
  status: string
  revision: number
  cutover_at: number
  checkpoint: string
}
type LegacyToken = {
  id: number
  name: string
  owner_user_id: number
  threshold_usd: number
  configured: boolean
}
type MigrationList = {
  migrations?: Migration[] | null
  legacy_tokens?: LegacyToken[] | null
  legacy_quota_items?: MigrationCandidate[] | null
  truncated: boolean
}
export function MigrationsPanel(props: { catalog: Catalog }) {
  const { t } = useTranslation(),
    client = useQueryClient(),
    verification = useSecureVerification()
  const [batchPending, setBatchPending] = useState(false)
  const [selected, setSelected] = useState<Migration>(),
    [acceptAmbiguous, setAcceptAmbiguous] = useState(false)
  const query = useQuery({
    queryKey: ['alerts', 'migrations'],
    queryFn: () => alertRequest<MigrationList>('migrations'),
  })
  const imported = useMutation({
    mutationFn: (input: {
      token_id?: number
      kind?: string
      object_id?: string
    }) => alertRequest('migrations', 'POST', input),
    onSuccess: () => void client.invalidateQueries({ queryKey: ['alerts'] }),
  })
  const switched = useMutation({
    mutationFn: async () => {
      if (!selected) return false
      const operation = selected.executor === 'legacy' ? 'cutover' : 'rollback'
      const payload = {
        expected_revision: selected.revision,
        accept_ambiguous: acceptAmbiguous,
      }
      const preview = await alertRequest<{
        snapshot: { snapshot_hash: string; ambiguous: boolean }
        verification_context: AlertOperation['context']
      }>(`migrations/${selected.id}/${operation}`, 'POST', {
        ...payload,
        preview: true,
      })
      if (
        operation === 'cutover' &&
        preview.snapshot.ambiguous &&
        !acceptAmbiguous
      ) {
        throw new Error(t('Confirm the legacy baseline before cutover.'))
      }
      const proof = await verification.requestVerification({
        scope: 'alert.manage',
        context: preview.verification_context,
      })
      if (!proof) return false
      await alertRequest(
        `migrations/${selected.id}/${operation}`,
        'POST',
        { ...payload, snapshot_hash: preview.snapshot.snapshot_hash },
        proof.proof_token
      )
      return true
    },
    onSuccess: (changed) => {
      if (changed) {
        setSelected(undefined)
        void client.invalidateQueries({ queryKey: ['alerts'] })
      }
    },
  })
  if (query.isPending) return <LoadingState />
  if (query.isError) return <ErrorState onRetry={() => void query.refetch()} />
  // Empty Go slices can arrive as null; older responses can omit a list.
  const list = {
    ...query.data,
    migrations: query.data.migrations ?? [],
    legacy_tokens: query.data.legacy_tokens ?? [],
    legacy_quota_items: query.data.legacy_quota_items ?? [],
  }
  const legacyItems = [
    ...list.legacy_tokens.map((token) => ({
      kind: 'token_spend',
      object_id: String(token.id),
      name: token.name,
      owner_user_id: token.owner_user_id,
      threshold_usd: String(token.threshold_usd),
      configured: token.configured,
    })),
    ...list.legacy_quota_items,
  ]
  const identities = new Set(
    list.migrations.map((item) => `${item.kind}:${item.object_id}`)
  )
  const candidates = legacyItems.filter(
    (item) => !identities.has(`${item.kind}:${item.object_id}`)
  )
  const name = (migration: Migration) => {
    if (migration.owner_user_id === 0) return alertLabel(migration.kind, t)
    return (
      (migration.kind === 'token_spend'
        ? list.legacy_tokens.find(
            (token) => String(token.id) === migration.object_id
          )?.name
        : list.legacy_quota_items.find(
            (item) =>
              item.kind === migration.kind &&
              item.object_id === migration.object_id
          )?.name) || migration.object_id
    )
  }
  return (
    <div className='space-y-4'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Import preserves the old executor. Cutover requires covered nodes, enabled statistics and notification mode.'
        )}
      </p>
      <MigrationImport
        candidates={candidates}
        disabled={
          imported.isPending ||
          switched.isPending ||
          !props.catalog.encryption_configured
        }
        onPendingChange={setBatchPending}
      />
      <StaticDataTable
        data={candidates}
        getRowKey={(row) => `${row.kind}:${row.object_id}`}
        columns={[
          {
            id: 'name',
            header: t('Monitored object'),
            cell: (row) => (
              <div>
                {row.owner_user_id === 0 ? alertLabel(row.kind, t) : row.name}
                <p className='text-muted-foreground text-xs'>
                  {alertLabel(row.kind, t)}
                </p>
              </div>
            ),
          },
          {
            id: 'scope',
            header: t('Scope'),
            cell: (row) => {
              if (row.owner_user_id === 0) return t('System')
              return t(row.kind === 'token_spend' ? 'API Keys' : 'Account')
            },
          },
          {
            id: 'threshold',
            header: t('Threshold (USD)'),
            cell: (row) => row.threshold_usd,
          },
          {
            id: 'configured',
            header: t('Notification targets'),
            cell: (row) => t(row.configured ? 'Configured' : 'Not configured'),
          },
          {
            id: 'import',
            header: t('Actions'),
            cell: (row) => (
              <Button
                size='sm'
                variant='outline'
                disabled={
                  imported.isPending ||
                  batchPending ||
                  !props.catalog.encryption_configured
                }
                onClick={() =>
                  imported.mutate(
                    row.kind === 'token_spend'
                      ? { token_id: Number(row.object_id) }
                      : { kind: row.kind, object_id: row.object_id }
                  )
                }
              >
                {t('Import as observer')}
              </Button>
            ),
          },
        ]}
      />
      <StaticDataTable
        data={list.migrations}
        getRowKey={(row) => row.id}
        columns={[
          {
            id: 'name',
            header: t('Monitored object'),
            cell: (row) => (
              <div>
                {name(row)}
                <p className='text-muted-foreground text-xs'>
                  {alertLabel(row.kind, t)}
                </p>
              </div>
            ),
          },
          {
            id: 'executor',
            header: t('Executor'),
            cell: (row) =>
              t(
                row.executor === 'legacy'
                  ? 'Legacy executor'
                  : 'Unified executor'
              ),
          },
          {
            id: 'status',
            header: t('Status'),
            cell: (row) => (
              <span>
                {alertLabel(row.status, t)}
                {row.executor === 'unified' && row.cutover_at > Date.now() && (
                  <span className='text-muted-foreground block text-xs'>
                    {t('Scheduled cutover: {{time}}', {
                      time: new Date(row.cutover_at).toISOString(),
                    })}
                  </span>
                )}
              </span>
            ),
          },
          {
            id: 'action',
            header: t('Actions'),
            cell: (row) => (
              <Button
                size='sm'
                variant='outline'
                disabled={
                  batchPending ||
                  (row.executor === 'legacy' &&
                    (!props.catalog.enabled || props.catalog.shadow_mode))
                }
                onClick={() => {
                  setAcceptAmbiguous(false)
                  setSelected(row)
                }}
              >
                {t(row.executor === 'legacy' ? 'Cut over' : 'Roll back')}
              </Button>
            ),
          },
        ]}
      />
      {list.truncated && (
        <p>
          {t('More legacy objects are available through authorized queries.')}
        </p>
      )}
      <ConfirmDialog
        open={!!selected}
        onOpenChange={(open) => {
          if (!open && !switched.isPending) setSelected(undefined)
        }}
        title={t(selected?.executor === 'legacy' ? 'Cut over' : 'Roll back')}
        desc={t(
          selected?.kind === 'key_hour' || selected?.kind === 'key_day'
            ? 'Natural-period takeover starts at the next boundary. The legacy executor handles the current period.'
            : 'Checkpoint transfer preserves unnotified usage. Pending deliveries must drain before rollback.'
        )}
        isLoading={switched.isPending}
        handleConfirm={() => switched.mutate()}
      >
        {selected?.executor === 'legacy' && (
          <div className='flex gap-2'>
            <Switch
              id='alert-baseline-confirm'
              checked={acceptAmbiguous}
              onCheckedChange={setAcceptAmbiguous}
            />
            <Label htmlFor='alert-baseline-confirm'>
              {t(
                'Accept the recorded legacy baseline; historical delivery cannot be proven.'
              )}
            </Label>
          </div>
        )}
      </ConfirmDialog>
      <SecureVerificationDialog {...verification.dialogProps} />
    </div>
  )
}

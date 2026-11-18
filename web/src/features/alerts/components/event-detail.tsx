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
import { CopyButton } from '@/components/copy-button'
import { StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { toIntlLocale } from '@/i18n/languages'

import { alertRequest } from '../api'
import { formatAlertEventSummary } from '../lib/event-summary'
import { alertLabel } from '../lib/labels'
import type {
  AlertEvent,
  EventDetail,
  AlertJob,
  AlertTarget,
  AlertOperation,
} from '../types'
import { EvidenceSummary } from './evidence-summary'

export function EventDetailDialog(props: {
  event: AlertEvent
  canManage: boolean
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const client = useQueryClient()
  const query = useQuery({
    queryKey: ['alerts', 'event', props.event.id],
    queryFn: () => alertRequest<EventDetail>(`events/${props.event.id}`),
    refetchInterval: 10000,
  })
  const verification = useSecureVerification()
  const [retrying, setRetrying] = useState<{
    job: AlertJob
    target: AlertTarget
    proof: AlertOperation['context']
  } | null>(null)
  const prepare = useMutation({
    mutationFn: async (job: AlertJob) => {
      const result = await alertRequest<{
        target: AlertTarget
        verification_context: AlertOperation['context']
      }>(`jobs/${job.id}/retry`, 'POST', { preview: true })
      setRetrying({
        job,
        target: result.target,
        proof: result.verification_context,
      })
    },
  })
  const retry = useMutation({
    mutationFn: async () => {
      if (!retrying) return false
      let proof: string | undefined
      if (retrying.job.kind === 'action') {
        const result = await verification.requestVerification({
          scope: 'alert.manage',
          context: retrying.proof,
        })
        if (!result) return false
        proof = result.proof_token
      }
      await alertRequest(
        `jobs/${retrying.job.id}/retry`,
        'POST',
        {
          target_id: retrying.target.id,
          target_revision: retrying.target.revision,
        },
        proof
      )
      return true
    },
    onSuccess: (saved) => {
      if (saved) {
        setRetrying(null)
        void client.invalidateQueries({ queryKey: ['alerts'] })
      }
    },
  })
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={props.event.target_name}
      description={formatAlertEventSummary(
        props.event.summary,
        t,
        locale,
        query.data?.config.quota_unit || undefined
      )}
      contentClassName='sm:max-w-4xl'
    >
      <div className='flex items-center gap-2 text-sm'>
        <span>
          {t('Event ID')}: {props.event.id}
        </span>
        <CopyButton value={String(props.event.id)} />
        <Badge>{props.event.severity}</Badge>
      </div>
      {query.isPending && <LoadingState />}
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      {query.data && (
        <EvidenceSummary
          evidence={query.data.event.evidence}
          config={query.data.config}
        />
      )}
      {query.data && (
        <StaticDataTable
          data={query.data.jobs}
          getRowKey={(job) => job.id}
          columns={[
            {
              id: 'target',
              header: t('Notification targets'),
              cell: (job) => (
                <span className='break-all'>
                  {job.target_name || '—'}
                  <span className='text-muted-foreground block text-xs'>
                    {job.target_hint}
                  </span>
                </span>
              ),
            },
            {
              id: 'kind',
              header: t('Type'),
              cell: (job) => alertLabel(job.action || job.kind, t),
            },
            {
              id: 'status',
              header: t('Status'),
              cell: (job) => (
                <Badge variant='outline'>{alertLabel(job.status, t)}</Badge>
              ),
            },
            {
              id: 'error',
              header: t('Result'),
              cell: (job) =>
                job.last_error ? alertLabel(job.last_error, t) : '—',
            },
            {
              id: 'retry',
              header: t('Actions'),
              cell: (job) =>
                query.data?.can_manage &&
                ['failed', 'blocked'].includes(job.status) ? (
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={prepare.isPending || retry.isPending}
                    onClick={() => prepare.mutate(job)}
                  >
                    {t('Retry')}
                  </Button>
                ) : null,
            },
          ]}
          emptyContent={t('No delivery jobs')}
        />
      )}
      <ConfirmDialog
        open={!!retrying}
        onOpenChange={(open) => {
          if (!open && !retry.isPending) setRetrying(null)
        }}
        title={t('Retry')}
        desc={
          retrying?.job.kind === 'action'
            ? t('The action will recheck current conditions and authorization.')
            : t(
                'Retry delivery using the current notification target revision.'
              )
        }
        isLoading={retry.isPending}
        handleConfirm={() => retry.mutate()}
      >
        <p className='break-all'>
          {retrying?.target.name} {retrying?.target.address_hint}
        </p>
      </ConfirmDialog>
      <SecureVerificationDialog {...verification.dialogProps} />
      <details className='mt-4 text-sm'>
        <summary>{t('Captured evidence')}</summary>
        <pre className='bg-muted mt-2 max-h-80 overflow-auto rounded p-3 break-all whitespace-pre-wrap'>
          {props.event.evidence}
        </pre>
      </details>
    </Dialog>
  )
}

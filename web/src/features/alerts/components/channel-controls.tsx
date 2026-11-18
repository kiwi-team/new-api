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
import { Dialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'

import { alertRequest, getAlertChannels } from '../api'
import { alertLabel } from '../lib/labels'
import type { AlertOperation, ChannelOption } from '../types'

export function ChannelControls() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const verification = useSecureVerification()
  const [selected, setSelected] = useState<ChannelOption>()
  const [models, setModels] = useState<ChannelOption>()
  const query = useQuery({
    queryKey: ['alerts', 'channels'],
    queryFn: getAlertChannels,
  })
  const change = useMutation({
    mutationFn: async () => {
      if (!selected) return
      const payload = {
        protection_mode:
          selected.protection_mode === 'scoped' ? 'legacy' : 'scoped',
        expected_revision: selected.protection_revision ?? 0,
      }
      const preview = await alertRequest<{
        verification_context: AlertOperation['context']
      }>(`channels/${selected.id}/protection`, 'POST', {
        ...payload,
        preview: true,
      })
      const proof = await verification.requestVerification({
        scope: 'alert.manage',
        context: preview.verification_context,
      })
      if (!proof) return
      await alertRequest(
        `channels/${selected.id}/protection`,
        'POST',
        payload,
        proof.proof_token
      )
    },
    onSuccess: () => {
      setSelected(undefined)
      void client.invalidateQueries({ queryKey: ['alerts'] })
    },
  })
  if (query.isError) return <ErrorState onRetry={() => void query.refetch()} />
  return (
    <>
      <p className='text-muted-foreground mb-3 text-sm'>
        {t(
          'Scoped protection keeps model failures local. Legacy protection may disable the whole channel.'
        )}
      </p>
      <StaticDataTable
        data={query.data ?? []}
        getRowKey={(row) => row.id}
        columns={[
          {
            id: 'channel',
            header: t('Channel'),
            cell: (row) => <span className='break-all'>{row.name}</span>,
          },
          {
            id: 'mode',
            header: t('Protection mode'),
            cell: (row) => (
              <Badge variant='outline'>
                {alertLabel(row.protection_mode ?? 'legacy', t)}
              </Badge>
            ),
          },
          {
            id: 'models',
            header: t('Models'),
            cell: (row) => (
              <Button
                size='sm'
                variant='outline'
                onClick={() => setModels(row)}
              >
                {t('Manage models')}
              </Button>
            ),
          },
          {
            id: 'change',
            header: t('Actions'),
            cell: (row) => (
              <Button
                size='sm'
                variant='outline'
                onClick={() => setSelected(row)}
              >
                {t('Change protection mode')}
              </Button>
            ),
          },
        ]}
        emptyContent={t('No channels')}
      />
      <ConfirmDialog
        open={!!selected}
        onOpenChange={(open) => {
          if (!open && !change.isPending) setSelected(undefined)
        }}
        title={t('Change protection mode')}
        desc={
          <>
            <p>{selected?.name}</p>
            <p>
              {t(
                'Scoped protection keeps model failures local. Legacy protection may disable the whole channel.'
              )}
            </p>
          </>
        }
        isLoading={change.isPending}
        handleConfirm={() => change.mutate()}
      />
      {models && (
        <ModelControlsDialog
          channel={models}
          onClose={() => setModels(undefined)}
        />
      )}
      <SecureVerificationDialog {...verification.dialogProps} />
    </>
  )
}

type ModelControlData = {
  name: string
  models: string[]
  config_version: string
  controls: { model_key: string; manual_paused: boolean; revision: number }[]
  holds: { target_type: string; model_key: string; target_name: string }[]
}
function ModelControlsDialog(props: {
  channel: ChannelOption
  onClose: () => void
}) {
  const { t } = useTranslation(),
    client = useQueryClient(),
    verification = useSecureVerification()
  const [selected, setSelected] = useState<{
    model: string
    paused: boolean
    revision: number
  }>()
  const query = useQuery({
    queryKey: ['alerts', 'model-controls', props.channel.id],
    queryFn: () =>
      alertRequest<ModelControlData>(`channels/${props.channel.id}/models`),
    refetchInterval: 10000,
  })
  const change = useMutation({
    mutationFn: async () => {
      if (!selected || !query.data) return false
      const payload = {
        model: selected.model,
        paused: !selected.paused,
        expected_revision: selected.revision,
        config_version: query.data.config_version,
      }
      const preview = await alertRequest<{
        verification_context: AlertOperation['context']
      }>(`channels/${props.channel.id}/models/control`, 'POST', {
        ...payload,
        preview: true,
      })
      const proof = await verification.requestVerification({
        scope: 'alert.manage',
        context: preview.verification_context,
      })
      if (!proof) return false
      await alertRequest(
        `channels/${props.channel.id}/models/control`,
        'POST',
        payload,
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
  const rows = [
    ...new Set([
      ...(query.data?.models ?? []),
      ...(query.data?.controls.map((row) => row.model_key) ?? []),
      ...(query.data?.holds.map((row) => row.model_key) ?? []),
    ]),
  ]
    .filter((model) => model && !model.includes('*'))
    .map((model) => {
      const control = query.data?.controls.find(
        (row) => row.model_key === model
      )
      const holds =
        query.data?.holds.filter(
          (hold) => hold.target_type === 'channel' || hold.model_key === model
        ) ?? []
      return {
        model,
        paused: control?.manual_paused ?? false,
        revision: control?.revision ?? 0,
        holds,
      }
    })
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={props.channel.name}
      description={t('Manual control and alert isolation remain independent.')}
      contentClassName='sm:max-w-3xl'
    >
      {query.isPending && <LoadingState />}
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      <StaticDataTable
        data={rows}
        getRowKey={(row) => row.model}
        columns={[
          { id: 'model', header: t('Models'), cell: (row) => row.model },
          {
            id: 'manual',
            header: t('Manual control'),
            cell: (row) => t(row.paused ? 'Paused' : 'Enabled'),
          },
          {
            id: 'holds',
            header: t('Automatic disable records'),
            cell: (row) =>
              row.holds.map((hold) => hold.target_name).join(', ') || t('None'),
          },
          {
            id: 'actions',
            header: t('Actions'),
            cell: (row) => (
              <Button
                size='sm'
                variant='outline'
                onClick={() => setSelected(row)}
              >
                {t(row.paused ? 'Resume' : 'Pause')}
              </Button>
            ),
          },
        ]}
      />
      <ConfirmDialog
        open={!!selected}
        onOpenChange={(open) => {
          if (!open && !change.isPending) setSelected(undefined)
        }}
        title={t('Change model availability')}
        desc={selected?.model ?? ''}
        isLoading={change.isPending}
        handleConfirm={() => change.mutate()}
      />
      <SecureVerificationDialog {...verification.dialogProps} />
    </Dialog>
  )
}

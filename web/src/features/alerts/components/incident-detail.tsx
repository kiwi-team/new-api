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
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { toIntlLocale } from '@/i18n/languages'

import { alertRequest } from '../api'
import { formatAlertEventSummary } from '../lib/event-summary'
import { alertLabel } from '../lib/labels'
import type { AlertEvent, AlertHold, AlertIncident } from '../types'

export function IncidentDetailDialog(props: {
  incident: AlertIncident
  canManage: boolean
  onClose: () => void
  onEvent: (event: AlertEvent) => void
}) {
  const { t, i18n } = useTranslation(),
    client = useQueryClient()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [closing, setClosing] = useState(false),
    [reason, setReason] = useState('')
  const query = useQuery({
    queryKey: ['alerts', 'incident', props.incident.id],
    queryFn: () =>
      alertRequest<{
        incident: AlertIncident
        can_manage: boolean
        events: AlertEvent[]
        holds: AlertHold[]
      }>(`incidents/${props.incident.id}`),
    refetchInterval: 10000,
  })
  const operation = useMutation({
    mutationFn: () =>
      alertRequest(`incidents/${props.incident.id}/close`, 'POST', {
        reason: reason.trim(),
      }),
    onSuccess: () => {
      setClosing(false)
      void client.invalidateQueries({ queryKey: ['alerts'] })
    },
  })
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose()
      }}
      title={props.incident.target_name}
      description={t('Incident lifecycle')}
      contentClassName='sm:max-w-4xl'
    >
      {query.isPending && <LoadingState />}
      {query.isError && <ErrorState onRetry={() => void query.refetch()} />}
      {query.data && (
        <div className='space-y-4 text-sm'>
          <p>
            {alertLabel(query.data.incident.status, t)} /{' '}
            {alertLabel(query.data.incident.handling_status, t)}
          </p>
          {query.data.incident.close_reason && (
            <p className='break-words'>
              {t('Close reason')}: {query.data.incident.close_reason}
            </p>
          )}
          <StaticDataTable
            data={query.data.events}
            getRowKey={(event) => event.id}
            columns={[
              {
                id: 'kind',
                header: t('Event'),
                cell: (event) => alertLabel(event.kind, t),
              },
              {
                id: 'summary',
                header: t('Captured evidence'),
                cell: (event) =>
                  formatAlertEventSummary(event.summary, t, locale),
              },
              {
                id: 'details',
                header: t('Actions'),
                cell: (event) => (
                  <Button
                    size='sm'
                    variant='outline'
                    onClick={() => props.onEvent(event)}
                  >
                    {t('Details')}
                  </Button>
                ),
              },
            ]}
          />
          {query.data.holds.length > 0 && (
            <>
              <h3 className='font-medium'>{t('Automatic disable records')}</h3>
              <StaticDataTable
                data={query.data.holds}
                getRowKey={(hold) => hold.id}
                columns={[
                  {
                    id: 'name',
                    header: t('Monitored object'),
                    cell: (hold) => hold.target_name,
                  },
                  {
                    id: 'status',
                    header: t('Status'),
                    cell: (hold) => t(hold.active ? 'Active' : 'Released'),
                  },
                  {
                    id: 'probe',
                    header: t('Recovery'),
                    cell: (hold) =>
                      alertLabel(hold.last_probe_status || 'manual', t),
                  },
                ]}
              />
            </>
          )}
          {query.data.can_manage &&
            query.data.incident.handling_status !== 'closed' && (
              <Button variant='outline' onClick={() => setClosing(true)}>
                {t('Close incident')}
              </Button>
            )}
        </div>
      )}
      <ConfirmDialog
        open={closing}
        onOpenChange={(open) => {
          if (!operation.isPending) setClosing(open)
        }}
        title={t('Close incident')}
        desc={t(
          'Closing an incident keeps active isolation and future detection.'
        )}
        disabled={!reason.trim()}
        isLoading={operation.isPending}
        handleConfirm={() => operation.mutate()}
      >
        <Label htmlFor='alert-close-reason'>{t('Close reason')}</Label>
        <Textarea
          id='alert-close-reason'
          maxLength={1000}
          value={reason}
          onChange={(event) => setReason(event.target.value)}
        />
      </ConfirmDialog>
    </Dialog>
  )
}

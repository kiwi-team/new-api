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
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { api } from '@/lib/api'

import { alertRequest } from '../api'
import type { AlertRule } from '../types'
import { EvidenceSummary } from './evidence-summary'

type ReportPage = { report: { has_more_keys: boolean; next_token_id: number } }
type ReportParams = {
  start: number
  end: number
  target_id: number
  after_token_id: number
}

export function ReportQuery({ rule }: { rule: AlertRule }) {
  const { t } = useTranslation()
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const [target, setTarget] = useState(rule.targets?.[0]?.id ?? 0)
  const query = useMutation({
    mutationFn: (params: ReportParams) =>
      alertRequest<ReportPage>(
        `rules/${rule.id}/report?${new URLSearchParams(Object.entries(params).map(([key, value]) => [key, String(value)]))}`
      ),
  })
  const exported = useMutation({
    mutationFn: async () => {
      if (!query.variables) return
      // The existing admin CSV helper is Root-only and cannot express rule
      // ownership or the report cursor. Use the same browser download pattern.
      const response = await api.get<Blob>(
        `/api/alerts/rules/${rule.id}/report`,
        { params: { ...query.variables, format: 'csv' }, responseType: 'blob' }
      )
      const url = window.URL.createObjectURL(response.data)
      const link = document.createElement('a')
      link.href = url
      link.download = 'alert-report.csv'
      link.click()
      window.URL.revokeObjectURL(url)
    },
  })
  const busy = query.isPending || exported.isPending
  const reset = () => {
    query.reset()
    exported.reset()
  }
  return (
    <section className='space-y-3 border-t pt-4'>
      <h3 className='font-medium'>{t('Query historical report')}</h3>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Select up to 31 days in your local time. Historical queries do not send notifications. CSV exports contain the current page and complete totals.'
        )}
      </p>
      <form
        className='grid gap-3 sm:grid-cols-2'
        onSubmit={(event) => {
          event.preventDefault()
          query.mutate({
            start: new Date(start).getTime(),
            end: new Date(end).getTime(),
            target_id: target,
            after_token_id: -1,
          })
        }}
      >
        <div className='space-y-2'>
          <Label htmlFor='alert-report-start'>{t('Start Time')}</Label>
          <Input
            id='alert-report-start'
            type='datetime-local'
            required
            value={start}
            disabled={busy}
            onChange={(event) => {
              setStart(event.target.value)
              reset()
            }}
          />
        </div>
        <div className='space-y-2'>
          <Label htmlFor='alert-report-end'>{t('End Time')}</Label>
          <Input
            id='alert-report-end'
            type='datetime-local'
            required
            value={end}
            disabled={busy}
            onChange={(event) => {
              setEnd(event.target.value)
              reset()
            }}
          />
        </div>
        <div className='space-y-2'>
          <Label htmlFor='alert-report-target'>{t('Monitored object')}</Label>
          <NativeSelect
            id='alert-report-target'
            value={String(target)}
            disabled={busy}
            onChange={(event) => {
              setTarget(Number(event.target.value))
              reset()
            }}
          >
            {rule.targets?.map((item) => (
              <NativeSelectOption
                key={`${item.type}:${item.id}`}
                value={String(item.id)}
              >
                {item.name}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </div>
        <div className='flex items-end gap-2'>
          <Button type='submit' disabled={busy || !start || !end}>
            {t('Query')}
          </Button>
          <Button
            type='button'
            variant='outline'
            disabled={busy || !query.data}
            onClick={() => exported.mutate()}
          >
            {t('Export CSV')}
          </Button>
        </div>
      </form>
      {query.isError && <p role='alert'>{query.error.message}</p>}
      {exported.isError && <p role='alert'>{t('Export failed')}</p>}
      {query.data && (
        <>
          <EvidenceSummary
            evidence={JSON.stringify({ measurement: query.data })}
            config={rule.config}
          />
          <div className='flex gap-2'>
            <Button
              variant='outline'
              disabled={busy || query.variables?.after_token_id === -1}
              onClick={() =>
                query.variables &&
                query.mutate({ ...query.variables, after_token_id: -1 })
              }
            >
              {t('First page')}
            </Button>
            <Button
              variant='outline'
              disabled={busy || !query.data.report.has_more_keys}
              onClick={() =>
                query.variables &&
                query.mutate({
                  ...query.variables,
                  after_token_id: query.data.report.next_token_id,
                })
              }
            >
              {t('Next')}
            </Button>
          </div>
        </>
      )}
    </section>
  )
}

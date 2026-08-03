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
import { useQuery } from '@tanstack/react-query'
import { Download, Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table'
import { DateTimePicker } from '@/components/datetime-picker'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { useDebounce } from '@/hooks/use-debounce'
import { cn } from '@/lib/utils'

import { exportSelfBillCsv, getBillTokenOptions, getSelfBill } from '../api'
import {
  DEFAULT_BILL_RANGE_DAYS,
  daysAgo,
  formatAmount,
  formatCount,
} from '../lib'
import type { BillQueryParams } from '../types'

const ALL_KEYS = 'all'

export function BillQueryTab() {
  const { t } = useTranslation()
  const [startTime, setStartTime] = useState<Date | undefined>(() =>
    daysAgo(DEFAULT_BILL_RANGE_DAYS)
  )
  const [endTime, setEndTime] = useState<Date | undefined>(() => new Date())
  const [tokenId, setTokenId] = useState<string>(ALL_KEYS)
  const [expandDate, setExpandDate] = useState(false)
  const [modelKeyword, setModelKeyword] = useState('')
  const [tokenKeyword, setTokenKeyword] = useState('')
  const debouncedTokenKeyword = useDebounce(tokenKeyword, 300)
  // Committed on "Query" so editing the filters does not refetch on every keystroke.
  const [query, setQuery] = useState<BillQueryParams | null>(() => ({
    start_timestamp: Math.floor(daysAgo(DEFAULT_BILL_RANGE_DAYS).getTime() / 1000),
    end_timestamp: Math.floor(Date.now() / 1000),
  }))

  const tokenOptionsQuery = useQuery({
    queryKey: ['bill', 'token-options', debouncedTokenKeyword],
    queryFn: () => getBillTokenOptions(debouncedTokenKeyword),
  })

  const billQuery = useQuery({
    queryKey: ['bill', 'self', query],
    queryFn: async () => {
      const result = await getSelfBill(query as BillQueryParams)
      if (!result.success) {
        toast.error(result.message || t('Failed to query the bill'))
        return null
      }
      return result.data ?? null
    },
    enabled: query !== null,
  })

  const buildParams = (): BillQueryParams | null => {
    if (!startTime || !endTime) {
      toast.warning(t('Please select a time range'))
      return null
    }
    const start = Math.floor(startTime.getTime() / 1000)
    const end = Math.floor(endTime.getTime() / 1000)
    if (start >= end) {
      toast.warning(t('The start time must be earlier than the end time'))
      return null
    }
    return {
      start_timestamp: start,
      end_timestamp: end,
      ...(tokenId === ALL_KEYS ? {} : { token_id: Number.parseInt(tokenId, 10) }),
      ...(expandDate ? { expand_date: true } : {}),
    }
  }

  const handleExport = async () => {
    const params = buildParams()
    if (!params) return
    try {
      const blob = await exportSelfBillCsv(params)
      const url = window.URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = 'settlement_bill.csv'
      link.click()
      window.URL.revokeObjectURL(url)
    } catch {
      toast.error(t('Export failed'))
    }
  }

  const bill = billQuery.data
  const items = useMemo(() => {
    const rows = bill?.items ?? []
    const kw = modelKeyword.trim().toLowerCase()
    if (!kw) return rows
    return rows.filter((item) => item.model_name.toLowerCase().includes(kw))
  }, [bill, modelKeyword])

  const filteredTotal = items.reduce(
    (total, item) => total + (item.total_amount || 0),
    0
  )

  const tokenItems = [
    { value: ALL_KEYS, label: t('All keys') },
    ...(tokenOptionsQuery.data ?? []).map((token) => ({
      value: String(token.id),
      label: token.username
        ? `${token.name || t('Unnamed')} (${token.username})`
        : token.name || t('Unnamed'),
    })),
  ]

  return (
    <div className='flex h-full min-h-0 flex-col gap-4'>
      <div className='flex flex-wrap items-end gap-3'>
        <div className='grid gap-1.5'>
          <Label>{t('Start Time')}</Label>
          <DateTimePicker value={startTime} onChange={setStartTime} />
        </div>
        <div className='grid gap-1.5'>
          <Label>{t('End Time')}</Label>
          <DateTimePicker value={endTime} onChange={setEndTime} />
        </div>
        <div className='grid gap-1.5'>
          <Label>{t('Filter by key')}</Label>
          <Select
            items={tokenItems}
            value={tokenId}
            onValueChange={(value) => setTokenId(value ?? ALL_KEYS)}
          >
            <SelectTrigger className='w-56'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <div className='p-1'>
                <Input
                  placeholder={t('Search keys')}
                  value={tokenKeyword}
                  onChange={(event) => setTokenKeyword(event.target.value)}
                />
              </div>
              {tokenItems.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className='flex items-center gap-2 pb-2'>
          <Switch
            id='bill-expand-date'
            checked={expandDate}
            onCheckedChange={setExpandDate}
          />
          <Label htmlFor='bill-expand-date'>{t('Expand by date')}</Label>
        </div>
        <Button
          className='mb-0.5'
          size='sm'
          onClick={() => {
            const params = buildParams()
            if (params) setQuery(params)
          }}
          disabled={billQuery.isFetching}
        >
          <Search className='h-4 w-4' />
          {t('Query')}
        </Button>
        <div className='mb-0.5'>
          <Input
            className='w-48'
            placeholder={t('Filter by model keyword')}
            value={modelKeyword}
            onChange={(event) => setModelKeyword(event.target.value)}
          />
        </div>
        <Button
          className='mb-0.5'
          variant='outline'
          size='sm'
          onClick={() => void handleExport()}
          disabled={!bill}
        >
          <Download className='h-4 w-4' />
          {t('Export CSV')}
        </Button>
      </div>

      <div className='min-h-0 flex-1 overflow-auto'>
        <StaticDataTable
          tableClassName='min-w-max'
          data={items}
          getRowKey={(item) =>
            item.date ? `${item.date}__${item.model_name}` : item.model_name
          }
          getRowClassName={(item) =>
            item.configured ? undefined : 'text-muted-foreground'
          }
          emptyContent={t('Select a time range and run the query')}
          emptyClassName='text-muted-foreground py-8'
          columns={[
            // The date column only exists when the server expanded the result.
            ...(bill?.expand_date
              ? [
                  {
                    id: 'date',
                    header: t('Date'),
                    cell: (item: (typeof items)[number]) => item.date ?? '-',
                  },
                ]
              : []),
            {
              id: 'model',
              header: t('Model name'),
              cellClassName: 'font-medium',
              cell: (item) => (
                <span className='flex items-center gap-2'>
                  {item.model_name}
                  {!item.configured && (
                    <Badge variant='secondary'>{t('No price configured')}</Badge>
                  )}
                </span>
              ),
            },
            {
              id: 'input-tokens',
              header: t('Input tokens'),
              cell: (item) => formatCount(item.input_tokens),
            },
            {
              id: 'output-tokens',
              header: t('Output tokens'),
              cell: (item) => formatCount(item.output_tokens),
            },
            {
              id: 'requests',
              header: t('Requests'),
              cell: (item) => formatCount(item.request_count),
            },
            {
              id: 'input-amount',
              header: t('Input amount'),
              cell: (item) => formatAmount(item.input_amount),
            },
            {
              id: 'output-amount',
              header: t('Output amount'),
              cell: (item) => formatAmount(item.output_amount),
            },
            {
              id: 'request-amount',
              header: t('Per-call amount'),
              cell: (item) =>
                item.request_amount > 0 ? formatAmount(item.request_amount) : '-',
            },
            {
              id: 'total-amount',
              header: t('Total amount'),
              cellClassName: (item) => cn(item.configured && 'font-semibold'),
              cell: (item) => formatAmount(item.total_amount),
            },
          ]}
        />
      </div>

      {bill && (
        <div className='bg-muted/40 rounded-lg px-4 py-3 text-right'>
          <span className='text-muted-foreground me-2 text-sm'>
            {modelKeyword.trim() ? t('Filtered total') : t('Total amount')}
          </span>
          <span className='text-lg font-semibold'>
            {formatAmount(filteredTotal)}
          </span>
        </div>
      )}
    </div>
  )
}

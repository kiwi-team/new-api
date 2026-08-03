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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCw, Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import { getSelfSettlementConfigs } from '../api'
import { formatPrice, siteListPrices } from '../lib'

type SelfSettlementTabProps = {
  /**
   * `prices` lists what the account is billed at; `discounts` contrasts the
   * site list price with the account's discount factor.
   */
  variant: 'prices' | 'discounts'
}

/** Both settlement tabs read the same endpoint and differ only in columns. */
export function SelfSettlementTab({ variant }: SelfSettlementTabProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [keyword, setKeyword] = useState('')

  const { data, isFetching } = useQuery({
    queryKey: ['bill', 'self-settlement-configs'],
    queryFn: getSelfSettlementConfigs,
  })

  const configs = useMemo(() => {
    const rows = data ?? []
    const kw = keyword.trim().toLowerCase()
    if (!kw) return rows
    return rows.filter((config) =>
      (config.model_name || '').toLowerCase().includes(kw)
    )
  }, [data, keyword])

  const discountColumn = {
    id: 'discount',
    header: t('Model discount'),
    cell: (config: (typeof configs)[number]) =>
      Number(config.discount ?? 1).toFixed(2),
  }

  return (
    <div className='flex h-full min-h-0 flex-col gap-4'>
      <div className='flex flex-wrap items-center gap-2'>
        <div className='relative'>
          <Search className='text-muted-foreground pointer-events-none absolute inset-y-0 start-2 my-auto h-4 w-4' />
          <Input
            className='w-56 ps-8'
            placeholder={t('Filter by model keyword')}
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
          />
        </div>
        <Button
          variant='outline'
          size='sm'
          disabled={isFetching}
          onClick={() =>
            void queryClient.invalidateQueries({
              queryKey: ['bill', 'self-settlement-configs'],
            })
          }
        >
          <RefreshCw className='h-4 w-4' />
          {t('Refresh')}
        </Button>
      </div>

      <div className='min-h-0 flex-1 overflow-auto'>
        <StaticDataTable
          tableClassName='min-w-max'
          data={configs}
          getRowKey={(config) => config.id}
          emptyContent={t('No settlement prices configured for this account')}
          emptyClassName='text-muted-foreground py-8'
          columns={
            variant === 'prices'
              ? [
                  {
                    id: 'model',
                    header: t('Model name'),
                    cellClassName: 'font-medium',
                    cell: (config) => config.model_name,
                  },
                  discountColumn,
                  {
                    id: 'input-price',
                    header: `${t('Input price')} (1M tokens)`,
                    cell: (config) => formatPrice(config.input_price),
                  },
                  {
                    id: 'output-price',
                    header: `${t('Output price')} (1M tokens)`,
                    cell: (config) => formatPrice(config.output_price),
                  },
                  {
                    id: 'request-price',
                    header: `${t('Per-call price')}`,
                    cell: (config) => formatPrice(config.request_price),
                  },
                ]
              : [
                  {
                    id: 'model',
                    header: t('Model name'),
                    cellClassName: 'font-medium',
                    cell: (config) => config.model_name,
                  },
                  {
                    id: 'list-input',
                    header: `${t('List input price (USD)')} (1M tokens)`,
                    cell: (config) => {
                      const { input } = siteListPrices(config)
                      return input === null ? '-' : formatPrice(input)
                    },
                  },
                  {
                    id: 'list-output',
                    header: `${t('List output price (USD)')} (1M tokens)`,
                    cell: (config) => {
                      const { output } = siteListPrices(config)
                      return output === null ? '-' : formatPrice(output)
                    },
                  },
                  discountColumn,
                ]
          }
        />
      </div>
    </div>
  )
}

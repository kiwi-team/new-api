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
import { getRouteApi } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { CustomerDiscountsTab } from './components/customer-discounts-tab'
import { OfficialPricesTab } from './components/official-prices-tab'

const route = getRouteApi('/_authenticated/pricing-center/')

export function PricingCenter() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const activeTab = search.tab ?? 'official'

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Pricing Center')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-4'>
          <Tabs
            value={activeTab}
            onValueChange={(tab) =>
              void navigate({ search: { tab: tab as 'official' | 'discount' } })
            }
          >
            <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
              <TabsTrigger value='official'>{t('Official prices')}</TabsTrigger>
              <TabsTrigger value='discount'>
                {t('Customer discounts')}
              </TabsTrigger>
            </TabsList>
          </Tabs>
          <div className='min-h-0 flex-1'>
            {activeTab === 'official' ? (
              <OfficialPricesTab />
            ) : (
              <CustomerDiscountsTab />
            )}
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

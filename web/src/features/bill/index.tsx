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
import { useAdminSidebarModuleEnabled } from '@/hooks/use-sidebar-config'

import { BillQueryTab } from './components/bill-query-tab'
import { SelfSettlementTab } from './components/self-settlement-tab'

const route = getRouteApi('/_authenticated/bill/')

export type BillTab = 'bill' | 'pricing' | 'discount'

export function BillPage() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const navigate = route.useNavigate()

  // Root decides per deployment which of the three views this page exposes.
  const billQueryEnabled = useAdminSidebarModuleEnabled('console', 'billQuery')
  const pricingEnabled = useAdminSidebarModuleEnabled('console', 'billPricing')
  const discountEnabled = useAdminSidebarModuleEnabled('console', 'billDiscount')

  const visibleTabs = [
    { id: 'bill' as const, label: t('Bill query'), enabled: billQueryEnabled },
    {
      id: 'pricing' as const,
      label: t('Settlement prices'),
      enabled: pricingEnabled,
    },
    {
      id: 'discount' as const,
      label: t('Settlement discounts'),
      enabled: discountEnabled,
    },
  ].filter((tab) => tab.enabled)

  const activeTab = visibleTabs.some((tab) => tab.id === search.tab)
    ? (search.tab as BillTab)
    : visibleTabs[0]?.id

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Bill')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex h-full min-h-0 flex-col gap-4'>
          {visibleTabs.length > 1 && (
            <Tabs
              value={activeTab}
              onValueChange={(tab) =>
                void navigate({ search: { tab: tab as BillTab } })
              }
            >
              <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
                {visibleTabs.map((tab) => (
                  <TabsTrigger key={tab.id} value={tab.id}>
                    {tab.label}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          )}
          <div className='min-h-0 flex-1'>
            {activeTab === 'bill' && <BillQueryTab />}
            {activeTab === 'pricing' && <SelfSettlementTab variant='prices' />}
            {activeTab === 'discount' && (
              <SelfSettlementTab variant='discounts' />
            )}
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

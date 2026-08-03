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
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'

import { ClientUserQuotaDialogs } from './components/client-user-quota-dialogs'
import { ClientUserQuotaPrimaryButtons } from './components/client-user-quota-primary-buttons'
import { ClientUserQuotaProvider } from './components/client-user-quota-provider'
import { ClientUserQuotaTable } from './components/client-user-quota-table'

export function ClientUserQuotaPage() {
  const { t } = useTranslation()

  return (
    <ClientUserQuotaProvider>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          {t('UID Budgets')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <ClientUserQuotaPrimaryButtons />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <ClientUserQuotaTable />
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ClientUserQuotaDialogs />
    </ClientUserQuotaProvider>
  )
}

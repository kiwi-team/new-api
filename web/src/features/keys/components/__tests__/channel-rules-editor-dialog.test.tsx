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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, test } from 'vitest'

import { ChannelRulesEditorDialog } from '../channel-rules-editor-dialog'

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

function renderDialog(randomType: 'order' | 'random' | 'race') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const freshAt = Date.now() + 60_000
  queryClient.setQueryData(
    ['channel-name-list'],
    [{ id: 1, name: 'channel-1' }],
    { updatedAt: freshAt }
  )
  queryClient.setQueryData(
    ['user-models'],
    { data: ['gpt-4o'] },
    { updatedAt: freshAt }
  )

  return render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <ChannelRulesEditorDialog
          open
          onOpenChange={() => undefined}
          onSave={() => undefined}
          value={JSON.stringify({
            'gpt-4o': {
              random_type: randomType,
              channels: [{ ids: [1], race_mode: 'random_n', race_count: 1 }],
            },
          })}
        />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

describe('ChannelRulesEditorDialog group strategies', () => {
  test.each(['order', 'random', 'race'] as const)(
    'shows the within-group strategy for %s routing',
    async (randomType) => {
      renderDialog(randomType)

      expect(
        await screen.findByLabelText('Within-group strategy')
      ).toBeVisible()
      expect(screen.getByLabelText('Random channel count')).toBeVisible()
    }
  )
})

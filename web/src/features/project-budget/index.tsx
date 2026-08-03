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
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'

import { ProjectBudgetDialogs } from './components/project-budget-dialogs'
import {
  ProjectBudgetProvider,
  useProjectBudget,
} from './components/project-budget-provider'
import { ProjectDashboardCards } from './components/project-dashboard-cards'
import { ProjectsTable } from './components/projects-table'

function CreateProjectButton() {
  const { t } = useTranslation()
  const { setOpen, setCurrentProject } = useProjectBudget()

  return (
    <Button
      size='sm'
      onClick={() => {
        setCurrentProject(null)
        setOpen('mutate')
      }}
    >
      <Plus className='h-4 w-4' />
      {t('Create')}
    </Button>
  )
}

export function ProjectBudgetPage() {
  const { t } = useTranslation()

  return (
    <ProjectBudgetProvider>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          {t('Project Budgets')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <CreateProjectButton />
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex h-full min-h-0 flex-col gap-4'>
            <ProjectDashboardCards />
            <div className='min-h-0 flex-1'>
              <ProjectsTable />
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ProjectBudgetDialogs />
    </ProjectBudgetProvider>
  )
}

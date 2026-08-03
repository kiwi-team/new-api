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
import React, { useState } from 'react'

import useDialogState from '@/hooks/use-dialog'

import type { Project, ProjectPlan } from '../types'

export type ProjectDialogType =
  | 'mutate'
  | 'plans'
  | 'plan-form'
  | 'allocations'
  | 'allocation-form'

type ProjectBudgetContextType = {
  open: ProjectDialogType | null
  setOpen: (dialog: ProjectDialogType | null) => void
  currentProject: Project | null
  setCurrentProject: React.Dispatch<React.SetStateAction<Project | null>>
  currentPlan: ProjectPlan | null
  setCurrentPlan: React.Dispatch<React.SetStateAction<ProjectPlan | null>>
  refreshTrigger: number
  triggerRefresh: () => void
}

const ProjectBudgetContext =
  React.createContext<ProjectBudgetContextType | null>(null)

export function ProjectBudgetProvider({
  children,
}: {
  children: React.ReactNode
}) {
  const [open, setOpen] = useDialogState<ProjectDialogType>(null)
  const [currentProject, setCurrentProject] = useState<Project | null>(null)
  const [currentPlan, setCurrentPlan] = useState<ProjectPlan | null>(null)
  const [refreshTrigger, setRefreshTrigger] = useState(0)

  const triggerRefresh = () => setRefreshTrigger((prev) => prev + 1)

  return (
    <ProjectBudgetContext
      value={{
        open,
        setOpen,
        currentProject,
        setCurrentProject,
        currentPlan,
        setCurrentPlan,
        refreshTrigger,
        triggerRefresh,
      }}
    >
      {children}
    </ProjectBudgetContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useProjectBudget = () => {
  const context = React.useContext(ProjectBudgetContext)

  if (!context) {
    throw new Error(
      'useProjectBudget has to be used within <ProjectBudgetProvider>'
    )
  }

  return context
}

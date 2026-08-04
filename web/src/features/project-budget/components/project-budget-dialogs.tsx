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
import { AllocationsDialog } from './allocations-dialog'
import { PlanFormDialog } from './plan-form-dialog'
import { PlansDialog } from './plans-dialog'
import { ProjectMutateDialog } from './project-mutate-dialog'
import { useProjectBudget } from './project-budget-provider'

/**
 * Project dialogs are nested: project → plans → allocations. Closing an inner
 * dialog returns to its parent instead of dropping the user back to the list.
 */
export function ProjectBudgetDialogs() {
  const {
    open,
    setOpen,
    currentProject,
    setCurrentProject,
    currentPlan,
    setCurrentPlan,
    triggerRefresh,
  } = useProjectBudget()

  return (
    <>
      {open === 'mutate' && (
        <ProjectMutateDialog
          open
          onOpenChange={(isOpen) => {
            if (isOpen) return
            setOpen(null)
            setCurrentProject(null)
          }}
          currentRow={currentProject}
          onSaved={triggerRefresh}
        />
      )}

      {open === 'plans' && currentProject && (
        <PlansDialog
          open
          onOpenChange={(isOpen) => {
            if (isOpen) return
            setOpen(null)
            setCurrentProject(null)
          }}
          project={currentProject}
        />
      )}

      {open === 'plan-form' && currentProject && (
        <PlanFormDialog
          open
          onOpenChange={(isOpen) => !isOpen && setOpen('plans')}
          projectId={currentProject.id}
          currentPlan={currentPlan}
          onSaved={() => {
            setCurrentPlan(null)
            setOpen('plans')
            triggerRefresh()
          }}
        />
      )}

      {open === 'allocations' && currentProject && currentPlan && (
        <AllocationsDialog
          open
          onOpenChange={(isOpen) => {
            if (isOpen) return
            setCurrentPlan(null)
            setOpen('plans')
          }}
          project={currentProject}
          plan={currentPlan}
          onChanged={triggerRefresh}
        />
      )}

      {open === 'plan-allocations' && currentProject && currentPlan && (
        <AllocationsDialog
          open
          onOpenChange={(isOpen) => {
            if (isOpen) return
            setCurrentPlan(null)
            setCurrentProject(null)
            setOpen(null)
          }}
          project={currentProject}
          plan={currentPlan}
          onChanged={triggerRefresh}
        />
      )}
    </>
  )
}

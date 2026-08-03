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
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'

import { deleteClientUserQuota } from '../api'
import { ClientUserQuotaMutateDialog } from './client-user-quota-mutate-dialog'
import { useClientUserQuota } from './client-user-quota-provider'
import { ProjectBudgetDialog } from './project-budget-dialog'

export function ClientUserQuotaDialogs() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow, setCurrentRow, triggerRefresh } =
    useClientUserQuota()

  const closeDialog = () => {
    setOpen(null)
    setCurrentRow(null)
  }

  const handleDelete = async () => {
    if (!currentRow) return
    const res = await deleteClientUserQuota(currentRow.id)
    closeDialog()
    if (!res.success) {
      toast.error(res.message || t('Delete failed'))
      return
    }
    toast.success(t('Deleted successfully'))
    triggerRefresh()
  }

  return (
    <>
      {open === 'mutate' && (
        <ClientUserQuotaMutateDialog
          open
          onOpenChange={(isOpen) => !isOpen && closeDialog()}
          currentRow={currentRow}
          onSaved={triggerRefresh}
        />
      )}

      {open === 'delete' && currentRow && (
        <ConfirmDialog
          open
          onOpenChange={(isOpen) => !isOpen && closeDialog()}
          title={t('Confirm deletion')}
          desc={t('Delete the budget record of {{uid}}?', {
            uid: currentRow.client_user_id,
          })}
          destructive
          handleConfirm={() => void handleDelete()}
        />
      )}

      {open === 'project-budget' && currentRow && (
        <ProjectBudgetDialog
          open
          onOpenChange={(isOpen) => !isOpen && closeDialog()}
          clientUserId={currentRow.client_user_id}
        />
      )}
    </>
  )
}

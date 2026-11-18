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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { Table } from '@tanstack/react-table'
import { Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'
import { AuthOperationError } from '@/lib/secure-verification'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { batchDeleteUsers } from '../api'
import type { User } from '../types'
import { useUsers } from './users-provider'

interface DataTableBulkActionsProps {
  table: Table<User>
}

export function DataTableBulkActions(props: DataTableBulkActionsProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isRoot = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const { requestVerification, verificationActive, triggerRefresh } = useUsers()
  const [pendingUsers, setPendingUsers] = useState<User[]>([])
  const selectedUsers = props.table
    .getFilteredSelectedRowModel()
    .rows.map((row) => row.original)
    .filter((user) => user.role < ROLE.SUPER_ADMIN)
  const description = t(
    'This will permanently delete {{count}} user(s) and all their API keys.',
    { count: pendingUsers.length }
  )
  const deletion = useMutation({
    mutationFn: async () => {
      const ids = pendingUsers.map((user) => user.id)
      const proof = await requestVerification({
        scope: 'admin.user.batch.delete',
        context: { user_ids: ids },
        title: t('Verify to delete users'),
        description,
      })
      if (!proof) return null
      return requireServerSuccess(
        await batchDeleteUsers(ids, proof.proof_token)
      )
    },
    onSuccess: (result) => {
      if (!result) return
      toast.success(
        t('Deleted {{deleted}} user(s); skipped {{skipped}}.', {
          deleted: result.data?.deleted ?? 0,
          skipped: result.data?.skipped ?? 0,
        })
      )
      setPendingUsers([])
      props.table.resetRowSelection()
      triggerRefresh()
      void queryClient.invalidateQueries({ queryKey: ['users'] })
      void queryClient.invalidateQueries({ queryKey: ['keys'] })
    },
    onError: (error) =>
      handleServerError(
        AuthOperationError.from(error),
        t('Failed to delete user')
      ),
  })

  if (!isRoot) return null

  return (
    <>
      <BulkActionsToolbar table={props.table} entityName='user'>
        <Button
          variant='destructive'
          size='sm'
          disabled={selectedUsers.length === 0 || deletion.isPending}
          onClick={() => setPendingUsers(selectedUsers)}
        >
          <Trash2 className='size-4' />
          {t('Delete selected users')}
        </Button>
      </BulkActionsToolbar>
      <ConfirmDialog
        open={pendingUsers.length > 0 && !verificationActive}
        onOpenChange={(open) => {
          if (!open && !deletion.isPending) setPendingUsers([])
        }}
        title={t('Delete {{count}} user(s)?', { count: pendingUsers.length })}
        desc={
          <>
            {description} {t('This action cannot be undone.')}
          </>
        }
        confirmText={t('Delete')}
        destructive
        isLoading={deletion.isPending}
        handleConfirm={() => deletion.mutate()}
      />
    </>
  )
}

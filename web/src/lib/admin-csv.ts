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
import { api } from '@/lib/api'

/** Entities behind the root-only `/api/admin/{export,import}` endpoints. */
export type AdminCsvEntity = 'channels' | 'tokens' | 'users'

export type AdminCsvImportResult = {
  success: boolean
  message?: string
  data?: unknown
}

/** Download the full table as CSV and hand it to the browser. */
export async function exportAdminCsv(entity: AdminCsvEntity): Promise<void> {
  const res = await api.get(`/api/admin/export/${entity}`, {
    responseType: 'blob',
  })
  const url = window.URL.createObjectURL(res.data as Blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `${entity}.csv`
  link.click()
  window.URL.revokeObjectURL(url)
}

export async function importAdminCsv(
  entity: AdminCsvEntity,
  file: File
): Promise<AdminCsvImportResult> {
  const formData = new FormData()
  formData.append('file', file)
  const res = await api.post(`/api/admin/import/${entity}`, formData)
  return res.data
}

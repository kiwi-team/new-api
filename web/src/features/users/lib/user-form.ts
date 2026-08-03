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
import { z } from 'zod'

import {
  type PermissionCatalog,
  type AdminPermissionMatrix,
  normalizeAdminPermissions,
} from '@/lib/admin-permissions'
import { quotaUnitsToDollars } from '@/lib/format'
import { ROLE } from '@/lib/roles'

import { DEFAULT_GROUP } from '../constants'
import { type UserFormData, type User } from '../types'

// ============================================================================
// Form Schema
// ============================================================================

export const userFormSchema = z.object({
  username: z.string().min(1, 'Username is required'),
  display_name: z.string().optional(),
  password: z.string().optional(),
  role: z.number().optional(),
  quota_dollars: z.number().min(0).optional(),
  group: z.string().optional(),
  remark: z.string().optional(),
  admin_permissions: z
    .record(z.string(), z.record(z.string(), z.boolean()))
    .optional(),
  // Root-only fields. `org_code` / `org_role` drive the menu and data scope
  // (see org.md); the rest are stored on the user row or inside `setting`.
  org_code: z.string().optional(),
  org_role: z.string().optional(),
  uid: z.string().optional(),
  related_uids: z.string().optional(),
  toio_registered: z.boolean().optional(),
  check_uid: z.boolean().optional(),
  group_discount: z.string().optional(),
  model_extra_discount: z.string().optional(),
})

export type UserFormValues = z.infer<typeof userFormSchema>

// ============================================================================
// Form Defaults
// ============================================================================

export const USER_FORM_DEFAULT_VALUES: UserFormValues = {
  username: '',
  display_name: '',
  password: '',
  role: 1, // Default to common user
  quota_dollars: 0,
  group: DEFAULT_GROUP,
  remark: '',
  org_code: '',
  org_role: 'member',
  uid: '',
  related_uids: '',
  toio_registered: false,
  check_uid: false,
  group_discount: '',
  model_extra_discount: '',
  // Filled against the backend catalog at render time; see UsersMutateDrawer.
  admin_permissions: {},
}

/** Roles a user can hold inside an organisation (`constant/org.go`). */
export const ORG_ROLES = ['member', 'leader', 'admin', 'mtuser'] as const

/**
 * Read a discount map out of the user's `setting` JSON for editing.
 *
 * Returns pretty-printed JSON so the textarea is readable, and `''` when the
 * key is absent so an untouched field stays empty rather than showing `{}`.
 */
export function readSettingJson(
  setting: string | undefined,
  key: 'group_discount' | 'model_extra_discount' | 'check_uid'
): string {
  if (!setting) return ''
  try {
    const parsed = JSON.parse(setting) as Record<string, unknown>
    const value = parsed?.[key]
    if (value === undefined || value === null) return ''
    return JSON.stringify(value, null, 2)
  } catch {
    return ''
  }
}

export function readSettingFlag(
  setting: string | undefined,
  key: 'check_uid'
): boolean {
  if (!setting) return false
  try {
    const parsed = JSON.parse(setting) as Record<string, unknown>
    return parsed?.[key] === true
  } catch {
    return false
  }
}

/**
 * Build the `setting` payload for an update.
 *
 * The backend merges only `group_discount`, `model_extra_discount` and
 * `check_uid` out of what is sent (`controller/user.go`), keeping every other
 * stored preference, so this deliberately sends just those three. An
 * unparseable textarea is dropped rather than sent, which would otherwise wipe
 * the stored map.
 */
export function buildUserSettingPayload(values: {
  group_discount?: string
  model_extra_discount?: string
  check_uid?: boolean
}): string {
  const setting: Record<string, unknown> = { check_uid: !!values.check_uid }

  for (const key of ['group_discount', 'model_extra_discount'] as const) {
    const raw = (values[key] ?? '').trim()
    if (!raw) continue
    try {
      setting[key] = JSON.parse(raw)
    } catch {
      // Leave the key out; the merge then preserves the stored value.
    }
  }

  return JSON.stringify(setting)
}

// ============================================================================
// Form Data Transformation
// ============================================================================

/**
 * Transform form data to API payload
 *
 * On update, `currentUser` must be the row being edited: `PUT /api/user/`
 * decodes into a fresh `model.User` and `EditWithTx` writes quota, uid,
 * related_uids, toio_registered and the org tags unconditionally, so anything
 * this payload omits is persisted as its zero value.
 *
 * `isRoot` gates the org/uid/discount inputs: only root can edit them, so a
 * non-root editor echoes the stored values back instead.
 */
export function transformFormDataToPayload(
  data: UserFormValues,
  userId?: number,
  catalog?: PermissionCatalog,
  currentUser?: User,
  isRoot = false
): UserFormData & { id?: number } {
  const payload: UserFormData & { id?: number } = {
    username: data.username,
    display_name: data.display_name || data.username,
    password: data.password || undefined,
  }

  const role = userId === undefined ? data.role || 1 : (data.role ?? 0)

  // Only send the permission matrix when the target is an admin and the catalog
  // is available; without the catalog we cannot build a full matrix, so we omit
  // the field (the backend then leaves existing permissions untouched).
  if (role >= ROLE.ADMIN && catalog) {
    payload.admin_permissions = normalizeAdminPermissions(
      data.admin_permissions as AdminPermissionMatrix | undefined,
      catalog
    )
  }

  // For create: only send required fields
  if (userId === undefined) {
    payload.role = role
  } else {
    // For update: quota is adjusted atomically via /api/user/manage, so the
    // stored value is echoed back rather than taken from the form.
    payload.group = data.group
    payload.remark = data.remark || undefined
    payload.id = userId
    payload.quota = currentUser?.quota ?? 0
    // Root-only fields fall back to the stored value so a non-root editor —
    // whose form never renders them — cannot blank them out.
    payload.uid = isRoot ? (data.uid ?? '') : (currentUser?.uid ?? '')
    payload.related_uids = isRoot
      ? (data.related_uids ?? '')
      : (currentUser?.related_uids ?? '')
    payload.toio_registered = isRoot
      ? data.toio_registered
        ? 1
        : 0
      : (currentUser?.toio_registered ?? 0)
    payload.org_code = isRoot
      ? (data.org_code ?? '')
      : (currentUser?.org_code ?? '')
    payload.org_role = isRoot
      ? (data.org_role ?? '')
      : (currentUser?.org_role ?? '')
    payload.setting = isRoot
      ? buildUserSettingPayload(data)
      : (currentUser?.setting ?? '')
  }

  return payload
}

/**
 * Transform user data to form defaults. The admin permission matrix is passed
 * through as-is (the backend already returns a full matrix); it is filled against
 * the catalog at render time in UsersMutateDrawer.
 */
export function transformUserToFormDefaults(user: User): UserFormValues {
  return {
    username: user.username,
    display_name: user.display_name,
    password: '',
    role: user.role,
    quota_dollars: quotaUnitsToDollars(user.quota),
    group: user.group || DEFAULT_GROUP,
    remark: user.remark || '',
    org_code: user.org_code || '',
    org_role: user.org_role || 'member',
    uid: user.uid || '',
    related_uids: user.related_uids || '',
    toio_registered: user.toio_registered === 1,
    check_uid: readSettingFlag(user.setting, 'check_uid'),
    group_discount: readSettingJson(user.setting, 'group_discount'),
    model_extra_discount: readSettingJson(user.setting, 'model_extra_discount'),
    admin_permissions: user.admin_permissions ?? {},
  }
}

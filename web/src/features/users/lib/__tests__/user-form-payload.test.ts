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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { transformFormDataToPayload } from '../user-form'
import type { UserFormValues } from '../user-form'
import type { User } from '../../types'

const formValues: UserFormValues = {
  username: 'alice',
  display_name: 'Alice',
  password: '',
  role: 1,
  quota_dollars: 12,
  group: 'default',
  remark: 'vip',
  org_code: 'acme',
  org_role: 'leader',
  uid: 'client-abc',
  related_uids: '["client-def"]',
  toio_registered: true,
  check_uid: true,
  group_discount: '{"default":0.8}',
  model_extra_discount: '',
  admin_permissions: undefined,
}

const storedUser = {
  id: 7,
  username: 'alice',
  display_name: 'Alice',
  quota: 5_000_000,
  used_quota: 10,
  request_count: 3,
  group: 'default',
  status: 1,
  role: 1,
  uid: 'client-abc',
  related_uids: '["client-def"]',
  toio_registered: 1,
  org_code: 'acme',
  org_role: 'leader',
  setting: '{"check_uid":true}',
} as User

describe('transformFormDataToPayload — update', () => {
  // `PUT /api/user/` decodes into a fresh model.User and EditWithTx writes
  // these columns unconditionally, so an omitted field is persisted as its
  // zero value. Editing a remark used to wipe the user's balance.
  test('echoes back every field the backend overwrites unconditionally', () => {
    // Non-root editor: the org/uid inputs are not rendered, so the stored
    // values must survive the round trip.
    const payload = transformFormDataToPayload(formValues, 7, undefined, storedUser)

    assert.equal(payload.quota, 5_000_000)
    assert.equal(payload.uid, 'client-abc')
    assert.equal(payload.related_uids, '["client-def"]')
    assert.equal(payload.toio_registered, 1)
    assert.equal(payload.org_code, 'acme')
    assert.equal(payload.org_role, 'leader')
    assert.equal(payload.setting, '{"check_uid":true}')
  })

  test('does not take quota from the form, which edits dollars only', () => {
    const payload = transformFormDataToPayload(
      { ...formValues, quota_dollars: 999 },
      7,
      undefined,
      storedUser
    )

    assert.equal(payload.quota, storedUser.quota)
  })

  test('sends the edited fields', () => {
    const payload = transformFormDataToPayload(
      { ...formValues, group: 'vip', remark: 'updated' },
      7,
      undefined,
      storedUser
    )

    assert.equal(payload.id, 7)
    assert.equal(payload.group, 'vip')
    assert.equal(payload.remark, 'updated')
  })
})

describe('transformFormDataToPayload — root editor', () => {
  test('sends the org, uid, and discount fields from the form', () => {
    const payload = transformFormDataToPayload(
      formValues,
      7,
      undefined,
      storedUser,
      true
    )

    assert.equal(payload.org_code, 'acme')
    assert.equal(payload.org_role, 'leader')
    assert.equal(payload.uid, 'client-abc')
    assert.equal(payload.toio_registered, 1)
    assert.deepEqual(JSON.parse(payload.setting ?? '{}'), {
      check_uid: true,
      group_discount: { default: 0.8 },
    })
  })

  test('clearing a root field sends the cleared value', () => {
    const payload = transformFormDataToPayload(
      { ...formValues, uid: '', org_code: '', toio_registered: false },
      7,
      undefined,
      storedUser,
      true
    )

    assert.equal(payload.uid, '')
    assert.equal(payload.org_code, '')
    assert.equal(payload.toio_registered, 0)
  })

  test('keeps a stored discount when the textarea holds invalid JSON', () => {
    // Sending a partial setting would make the backend merge `undefined` over
    // the stored map, so an unparseable field is omitted instead.
    const payload = transformFormDataToPayload(
      { ...formValues, group_discount: '{not json' },
      7,
      undefined,
      storedUser,
      true
    )

    assert.equal(
      Object.hasOwn(JSON.parse(payload.setting ?? '{}'), 'group_discount'),
      false
    )
  })
})

describe('transformFormDataToPayload — create', () => {
  test('omits the echo-back fields, which have no stored value yet', () => {
    const payload = transformFormDataToPayload(formValues, undefined)

    assert.equal(payload.id, undefined)
    assert.equal(payload.quota, undefined)
    assert.equal(payload.uid, undefined)
    assert.equal(payload.role, 1)
  })
})

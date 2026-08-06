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
import { describe, it } from 'node:test'

import { parseUserSettings } from '../format'

describe('profile settings parser', () => {
  it('returns an empty object for valid JSON that is not a settings object', () => {
    assert.deepEqual(parseUserSettings('null'), {})
    assert.deepEqual(parseUserSettings('["model"]'), {})
  })

  it('normalizes legacy model-limit maps without losing other settings', () => {
    assert.deepEqual(
      parseUserSettings(
        '{"notify_type":"email","model_limits":{"gpt-4o":true,"disabled":false}}'
      ),
      { notify_type: 'email', model_limits: ['gpt-4o'] }
    )
  })
})

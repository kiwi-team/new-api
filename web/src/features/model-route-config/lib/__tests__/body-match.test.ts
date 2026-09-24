import { describe, expect, test } from 'vitest'

import { parseBodyMatchDraft } from '../body-match'

describe('model route body match validation', () => {
  test('accepts a non-empty nested JSON object', () => {
    expect(parseBodyMatchDraft('{"thinking":{"type":"adaptive"}}')).toEqual({
      value: { thinking: { type: 'adaptive' } },
      error: null,
    })
  })

  test('treats a blank draft as no structured condition', () => {
    expect(parseBodyMatchDraft('   ')).toEqual({ value: null, error: null })
  })

  test.each(['{}', '[]', 'null', '"adaptive"'])(
    'rejects a non-empty-object condition: %s',
    (draft) => {
      expect(parseBodyMatchDraft(draft)).toEqual({
        value: null,
        error: 'non_empty_object_required',
      })
    }
  )

  test('rejects invalid JSON', () => {
    expect(parseBodyMatchDraft('{"thinking":')).toEqual({
      value: null,
      error: 'invalid_json',
    })
  })
})

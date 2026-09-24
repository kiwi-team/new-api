export type BodyMatch = Record<string, unknown>

export type BodyMatchParseResult =
  | { value: BodyMatch | null; error: null }
  | { value: null; error: 'invalid_json' | 'non_empty_object_required' }

export function parseBodyMatchDraft(draft: string): BodyMatchParseResult {
  const trimmed = draft.trim()
  if (!trimmed) {
    return { value: null, error: null }
  }

  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  } catch {
    return { value: null, error: 'invalid_json' }
  }

  if (
    typeof parsed !== 'object' ||
    parsed === null ||
    Array.isArray(parsed) ||
    Object.keys(parsed).length === 0
  ) {
    return { value: null, error: 'non_empty_object_required' }
  }

  return { value: parsed as BodyMatch, error: null }
}

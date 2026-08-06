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
import type { UserProfile, UserSettings } from '../types'

// ============================================================================
// Profile Formatting Utilities
// ============================================================================

/**
 * Parse user settings from JSON string
 */
export function parseUserSettings(settingsJson?: string): UserSettings {
  if (!settingsJson) return {}

  try {
    const parsed: unknown = JSON.parse(settingsJson)
    if (
      typeof parsed !== 'object' ||
      parsed === null ||
      Array.isArray(parsed)
    ) {
      return {}
    }

    const settings = { ...parsed } as Record<string, unknown>
    const rawModelLimits = settings.model_limits
    if (Array.isArray(rawModelLimits)) {
      settings.model_limits = [
        ...new Set(
          rawModelLimits
            .filter((value): value is string => typeof value === 'string')
            .map((value) => value.trim())
            .filter(Boolean)
        ),
      ]
    } else if (typeof rawModelLimits === 'string') {
      settings.model_limits = [
        ...new Set(
          rawModelLimits
            .split(',')
            .map((value) => value.trim())
            .filter(Boolean)
        ),
      ]
    } else if (typeof rawModelLimits === 'object' && rawModelLimits !== null) {
      settings.model_limits = Object.entries(rawModelLimits)
        .filter(([, enabled]) => enabled === true)
        .map(([model]) => model)
    } else {
      settings.model_limits = []
    }
    return settings as UserSettings
  } catch {
    return {}
  }
}

/**
 * Get display name or fallback to username
 */
export function getDisplayName(user?: UserProfile): string {
  if (!user) return ''
  return user.display_name || user.username
}

/**
 * Get user initials for avatar
 */
export function getUserInitials(user?: UserProfile): string {
  if (!user) return '?'
  const name = getDisplayName(user)
  if (!name) return '?'

  const parts = name.trim().split(/\s+/)
  if (parts.length >= 2) {
    return `${parts[0][0]}${parts[1][0]}`.toUpperCase()
  }
  return name.slice(0, 2).toUpperCase()
}

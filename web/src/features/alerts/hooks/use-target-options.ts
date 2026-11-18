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
import { useInfiniteQuery } from '@tanstack/react-query'
import { useState } from 'react'

import { useDebounce } from '@/hooks/use-debounce'

import { alertRequest } from '../api'
import type { AlertTarget } from '../types'

export function useTargetOptions(enabled: boolean) {
  const [search, setSearch] = useState('')
  const keyword = useDebounce(search.trim(), 300)
  const pendingSearch = keyword !== search.trim()
  const query = useInfiniteQuery({
    queryKey: ['alerts', 'targets', 'options', keyword],
    queryFn: async ({ pageParam }) => {
      const params = new URLSearchParams({ scope: 'all', limit: '200' })
      if (keyword) params.set('q', keyword)
      if (pageParam) params.set('before_id', String(pageParam))
      return (
        (await alertRequest<AlertTarget[] | null>(`targets?${params}`)) ?? []
      )
    },
    initialPageParam: 0,
    getNextPageParam: (last) =>
      last.length === 200 ? last.at(-1)?.id : undefined,
    enabled: enabled && !pendingSearch,
    meta: { errorToast: false },
  })
  return { query, setSearch, pendingSearch }
}

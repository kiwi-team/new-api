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
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxTrigger,
  ComboboxValue,
} from '@/components/ui/combobox'
import type { User } from '@/features/users/types'
import { useDebounce } from '@/hooks/use-debounce'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getUser, getUserOptionsPage } from '../api'

type Props = {
  id: string
  label: string
  value: number
  includeSelf?: boolean
  disabled?: boolean
  invalid?: boolean
  onValueChange: (value: number) => void
  onBlur?: () => void
}

function userOption(user: User) {
  const name = user.display_name || user.username
  const account =
    user.display_name && user.display_name !== user.username
      ? ` (${user.username})`
      : ''
  return { value: user.id, label: `${name}${account} (ID: ${user.id})` }
}

export function UserSelect(props: Props) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [keyword, setKeyword] = useState('')
  const [picked, setPicked] = useState<ReturnType<typeof userOption> | null>(
    null
  )
  const search = useDebounce(keyword.trim(), 300)
  const pendingSearch = search !== keyword.trim()
  const owner = useQuery({
    queryKey: ['users', 'selection', props.value],
    queryFn: async () => requireServerSuccess(await getUser(props.value)).data,
    enabled: props.value > 0,
  })
  const users = useInfiniteQuery({
    queryKey: ['users', 'options', search],
    queryFn: ({ pageParam }) => getUserOptionsPage(search, pageParam),
    initialPageParam: 1,
    getNextPageParam: (last, pages) =>
      pages.reduce((count, page) => count + page.items.length, 0) < last.total
        ? last.page + 1
        : undefined,
    enabled: open && !props.disabled && !pendingSearch,
    meta: { errorToast: false },
  })
  const options = pendingSearch
    ? []
    : (users.data?.pages ?? [])
        .flatMap((page) => page.items)
        .filter((user) => !user.DeletedAt)
        .map(userOption)
  const self = props.includeSelf ? { value: 0, label: t('My account') } : null
  if (self && !search && !pendingSearch) options.unshift(self)
  let selected = options.find((option) => option.value === props.value) ?? null
  if (self && props.value === 0) selected = self
  if (!selected && owner.data?.id === props.value) {
    selected = userOption(owner.data)
  }
  if (!selected && picked?.value === props.value) selected = picked

  let feedback = <ComboboxEmpty>{t('No matching items')}</ComboboxEmpty>
  if (pendingSearch || users.isPending) {
    feedback = (
      <div className='p-3'>
        <LoadingState inline size='sm' message={t('Loading...')} />
      </div>
    )
  } else if (users.isError) {
    feedback = (
      <ErrorState
        title={t('Failed to load users')}
        className='min-h-32 p-3'
        onRetry={() => void users.refetch()}
      />
    )
  }

  return (
    <Combobox
      items={options}
      value={selected}
      disabled={props.disabled}
      open={open}
      filter={null}
      inputValue={keyword}
      itemToStringLabel={(option) => option.label}
      isItemEqualToValue={(item, value) => item.value === value.value}
      onInputValueChange={(value, details) => {
        if (details.reason === 'input-change') setKeyword(value)
      }}
      onValueChange={(option) => {
        if (!option) return
        setPicked(option)
        props.onValueChange(option.value)
      }}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen)
        if (!nextOpen) setKeyword('')
      }}
    >
      <ComboboxTrigger
        id={props.id}
        aria-label={props.label}
        aria-invalid={props.invalid}
        disabled={props.disabled}
        onBlur={props.onBlur}
        render={<Button variant='outline' />}
        className='w-full min-w-0 justify-between font-normal'
      >
        <span className='min-w-0 truncate'>
          <ComboboxValue>
            {() =>
              selected?.label ??
              (props.value > 0 ? `#${props.value}` : t('Search users'))
            }
          </ComboboxValue>
        </span>
      </ComboboxTrigger>
      <ComboboxContent>
        <ComboboxInput
          showTrigger={false}
          placeholder={t('Filter by name or ID...')}
          aria-label={t('Filter by name or ID...')}
        />
        <ComboboxList>
          {options.map((option) => (
            <ComboboxItem key={option.value} value={option}>
              <span className='min-w-0 break-words'>{option.label}</span>
            </ComboboxItem>
          ))}
        </ComboboxList>
        {feedback}
        {users.hasNextPage && !users.isError && !pendingSearch && (
          <div className='p-1'>
            <Button
              variant='ghost'
              className='w-full'
              disabled={users.isFetchingNextPage}
              onClick={() => void users.fetchNextPage()}
            >
              {users.isFetchingNextPage
                ? t('Loading...')
                : t('Load more users')}
            </Button>
          </div>
        )}
      </ComboboxContent>
    </Combobox>
  )
}

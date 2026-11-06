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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { DropdownMenuItem } from '@/components/ui/dropdown-menu'
import { hasPermission } from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

// Reuse this entry across object pages so links retain ownership and IDs.
// Names remain display values and never become an authorization selector.
export function RelatedAlertsLink(props: {
  ownerID: number
  channelID?: number
  tokenID?: number
  menu?: boolean
}) {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  if (
    !user ||
    ((props.channelID || props.ownerID !== user.id) &&
      !hasPermission(user, 'alert', 'read_all'))
  ) {
    return null
  }
  const link = (
    <Link
      to='/alerts'
      search={{
        owner_user_id: props.ownerID,
        channel_id: props.channelID,
        token_id: props.tokenID,
      }}
    />
  )
  if (props.menu) {
    return (
      <DropdownMenuItem render={link}>{t('Alert center')}</DropdownMenuItem>
    )
  }
  return (
    <Link
      className='text-primary underline underline-offset-4'
      to='/alerts'
      search={{
        owner_user_id: props.ownerID,
        channel_id: props.channelID,
        token_id: props.tokenID,
      }}
    >
      {t('Alert center')}
    </Link>
  )
}

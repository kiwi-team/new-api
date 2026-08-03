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
import { useTranslation } from 'react-i18next'

import { errorPreview } from '../lib'
import type { MonitorRecord } from '../types'

type MonitorErrorListProps = {
  record: MonitorRecord
  limit?: number
  /** Internal view shows raw upstream error text */
  showSensitive?: boolean
}

export function MonitorErrorList({
  record,
  limit = 2,
  showSensitive = false,
}: MonitorErrorListProps) {
  const { t } = useTranslation()

  if (record.errorsTop.length === 0) {
    return (
      <p className='text-muted-foreground text-xs'>
        {t('No aggregated errors in this range.')}
      </p>
    )
  }

  return (
    <div className='space-y-1.5'>
      {record.errorsTop.slice(0, limit).map((error, index) => (
        <div
          // Buckets can repeat code+timestamp, so position disambiguates them.
          // eslint-disable-next-line react/no-array-index-key
          key={`${error.code}-${error.last}-${index}`}
          className='bg-muted/40 rounded-md px-2 py-1.5 text-xs'
        >
          <div className='text-muted-foreground flex items-center justify-between'>
            <span>{t('{{count}} times', { count: error.count })}</span>
            <span>
              {t('Last')} {error.last}
            </span>
          </div>
          <div className='mt-0.5 break-words'>
            {errorPreview(error, showSensitive, t)}
          </div>
        </div>
      ))}
    </div>
  )
}

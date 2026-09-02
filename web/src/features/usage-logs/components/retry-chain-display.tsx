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
import { GitBranch } from 'lucide-react'
import { useTranslation } from 'react-i18next'

interface RetryChainDisplayProps {
  chain: string
}

export function RetryChainDisplay(props: RetryChainDisplayProps) {
  const { t } = useTranslation()

  return (
    <span
      className='text-muted-foreground inline-flex min-w-0 items-center gap-1'
      aria-label={`${t('Retry Chain')}: ${props.chain}`}
    >
      <GitBranch
        className='size-3.5 shrink-0 text-amber-500'
        aria-hidden='true'
      />
      <span className='font-mono text-xs whitespace-nowrap'>{props.chain}</span>
    </span>
  )
}

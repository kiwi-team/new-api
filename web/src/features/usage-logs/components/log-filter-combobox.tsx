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

import {
  ComboboxInput,
  type ComboboxInputOption,
} from '@/components/ui/combobox-input'

interface LogFilterComboboxProps {
  options: ComboboxInputOption[]
  value: string
  onValueChange: (value: string) => void
  placeholder: string
  id?: string
  type?: React.HTMLInputTypeAttribute
}

export function LogFilterCombobox(props: LogFilterComboboxProps) {
  const { t } = useTranslation()

  return (
    <ComboboxInput
      options={props.options}
      value={props.value}
      onValueChange={props.onValueChange}
      placeholder={props.placeholder}
      id={props.id}
      emptyText={t('No matching items')}
      allowCustomValue
      clearable
      type={props.type}
      className='h-8 min-w-0 text-sm leading-5'
    />
  )
}

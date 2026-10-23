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
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { useDebounce } from '@/hooks/use-debounce'

import { getTargets, type Rule } from './api'

export function TargetPicker(props: {
  scope: Rule['scope']
  value: number[]
  onChange: (ids: number[]) => void
  disabled: boolean
}) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const term = useDebounce(search, 300)
  const kind = props.scope === 'user_model' ? 'user' : props.scope
  const query = useQuery({
    queryKey: ['gateway-limits', 'targets', kind, term],
    queryFn: () => getTargets(kind, term),
    enabled: !props.disabled,
    staleTime: 30_000,
  })
  const choices = new Map(
    (query.data ?? []).map((target) => [
      String(target.id),
      { value: String(target.id), label: `${target.name} (#${target.id})` },
    ])
  )
  for (const id of props.value) {
    if (!choices.has(String(id))) {
      choices.set(String(id), { value: String(id), label: `#${id}` })
    }
  }
  const options = [...choices.values()]
  return (
    <div className='space-y-2'>
      {!props.disabled && (
        <Input
          aria-label={t('Search targets by name or ID')}
          placeholder={t('Search targets by name or ID')}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          maxLength={64}
        />
      )}
      {props.scope === 'provider' ? (
        <MultiSelect
          id='limit-target'
          options={options}
          selected={props.value.map(String)}
          onChange={(values) => props.onChange(values.map(Number))}
          disabled={props.disabled || query.isError}
          maxVisibleChips={4}
          placeholder={t('Select a target')}
        />
      ) : (
        <Combobox
          id='limit-target'
          aria-label={t('Target')}
          options={options}
          value={props.value[0] ? String(props.value[0]) : ''}
          onValueChange={(value) =>
            props.onChange(value ? [Number(value)] : [])
          }
          disabled={props.disabled || query.isError}
          placeholder={t('Select a target')}
        />
      )}
      {query.isError && (
        <p role='alert' className='text-destructive text-sm'>
          {t('Failed to load targets')}
        </p>
      )}
    </div>
  )
}

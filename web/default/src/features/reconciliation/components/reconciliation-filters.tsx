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
import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useDebounce } from '@/hooks'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import dayjs from '@/lib/dayjs'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from '@/components/ui/combobox'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { DatePicker } from '@/components/date-picker'
import { searchModels } from '@/features/models/api'
import { getUser, searchUsers } from '@/features/users/api'
import type {
  ReconciliationFilters as FilterValues,
  ReconciliationGranularity,
} from '../types'

type RangePreset = '7' | '30' | '90' | 'custom'

interface SearchOption {
  label: string
  value: string
}

interface AsyncSearchComboboxProps {
  id: string
  value?: string
  selectedLabel?: string
  options: SearchOption[]
  isLoading: boolean
  placeholder: string
  emptyText: string
  onSearchChange: (value: string) => void
  onValueChange: (value?: string) => void
}

function AsyncSearchCombobox({
  id,
  value,
  selectedLabel,
  options,
  isLoading,
  placeholder,
  emptyText,
  onSearchChange,
  onValueChange,
}: AsyncSearchComboboxProps) {
  const labelMap = useMemo(
    () => new Map(options.map((option) => [option.value, option.label])),
    [options]
  )
  const items = useMemo(() => {
    const values = options.map((option) => option.value)
    if (value && !values.includes(value)) values.unshift(value)
    return values
  }, [options, value])
  const [inputValue, setInputValue] = useState(selectedLabel ?? value ?? '')

  return (
    <Combobox
      items={items}
      value={value ?? null}
      inputValue={inputValue}
      itemToStringLabel={(item) => labelMap.get(item) ?? selectedLabel ?? item}
      onInputValueChange={(next) => {
        setInputValue(next)
        onSearchChange(next)
      }}
      onValueChange={(next) => {
        const selected = next ?? undefined
        setInputValue(selected ? (labelMap.get(selected) ?? selected) : '')
        onSearchChange('')
        onValueChange(selected)
      }}
    >
      <ComboboxInput
        id={id}
        className='w-full min-w-48'
        placeholder={placeholder}
        showClear={Boolean(value || inputValue)}
      />
      <ComboboxContent>
        <ComboboxList>
          {(item: string) => (
            <ComboboxItem key={item} value={item}>
              {labelMap.get(item) ?? selectedLabel ?? item}
            </ComboboxItem>
          )}
        </ComboboxList>
        <ComboboxEmpty>{isLoading ? '…' : emptyText}</ComboboxEmpty>
      </ComboboxContent>
    </Combobox>
  )
}

function getRangePreset(startDate: string, endDate: string): RangePreset {
  const end = dayjs().startOf('day')
  if (endDate !== end.format('YYYY-MM-DD')) return 'custom'
  for (const days of [7, 30, 90] as const) {
    if (startDate === end.subtract(days - 1, 'day').format('YYYY-MM-DD')) {
      return String(days) as RangePreset
    }
  }
  return 'custom'
}

interface ReconciliationFiltersProps {
  value: FilterValues
  onChange: (patch: Partial<FilterValues>) => void
  onReset: () => void
}

export function ReconciliationFilters({
  value,
  onChange,
  onReset,
}: ReconciliationFiltersProps) {
  const { t } = useTranslation()
  const [userSearch, setUserSearch] = useState('')
  const [modelSearch, setModelSearch] = useState('')
  const debouncedUserSearch = useDebounce(userSearch, 350)
  const debouncedModelSearch = useDebounce(modelSearch, 350)

  const usersQuery = useQuery({
    queryKey: ['reconciliation-user-options', debouncedUserSearch],
    queryFn: () =>
      searchUsers({
        keyword: debouncedUserSearch.trim(),
        p: 1,
        page_size: 20,
      }),
  })
  const selectedUserQuery = useQuery({
    queryKey: ['reconciliation-selected-user', value.userId],
    queryFn: () => getUser(value.userId!),
    enabled: Boolean(value.userId),
  })
  const modelsQuery = useQuery({
    queryKey: ['reconciliation-model-options', debouncedModelSearch],
    queryFn: () =>
      searchModels({
        keyword: debouncedModelSearch.trim(),
        p: 1,
        page_size: 20,
      }),
  })

  const userOptions = useMemo<SearchOption[]>(() => {
    return (usersQuery.data?.data?.items ?? []).map((user) => ({
      value: String(user.id),
      label: `${user.username} (#${user.id})`,
    }))
  }, [usersQuery.data?.data?.items])
  const modelOptions = useMemo<SearchOption[]>(() => {
    const names = new Set(
      (modelsQuery.data?.data?.items ?? []).map((model) => model.model_name)
    )
    return Array.from(names).map((modelName) => ({
      value: modelName,
      label: modelName,
    }))
  }, [modelsQuery.data?.data?.items])
  const selectedUser = selectedUserQuery.data?.data
  const selectedUserLabel = selectedUser
    ? `${selectedUser.username} (#${selectedUser.id})`
    : value.userId
      ? `#${value.userId}`
      : undefined

  const activePreset = getRangePreset(value.startDate, value.endDate)
  const changePreset = (preset: string) => {
    if (preset === 'custom') return
    const days = Number(preset)
    const end = dayjs().startOf('day')
    onChange({
      startDate: end.subtract(days - 1, 'day').format('YYYY-MM-DD'),
      endDate: end.format('YYYY-MM-DD'),
    })
  }
  const changeDate = (field: 'startDate' | 'endDate', date?: Date) => {
    if (!date) return
    const nextDate = dayjs(date).format('YYYY-MM-DD')
    const nextStart = field === 'startDate' ? nextDate : value.startDate
    const nextEnd = field === 'endDate' ? nextDate : value.endDate
    if (dayjs(nextStart).isAfter(dayjs(nextEnd))) {
      toast.error(t('Start date must not be later than end date'))
      return
    }
    if (dayjs(nextEnd).diff(dayjs(nextStart), 'day') > 365) {
      toast.error(t('The date range cannot exceed 366 days'))
      return
    }
    onChange({ [field]: nextDate })
  }

  const granularityItems = [
    { value: 'day', label: t('Daily') },
    { value: 'month', label: t('Monthly') },
  ]

  return (
    <Card size='sm'>
      <CardHeader>
        <CardTitle>{t('Filters')}</CardTitle>
        <CardDescription>
          {t('Choose a date range, user, model, and aggregation interval.')}
        </CardDescription>
        <CardAction>
          <Button variant='ghost' size='sm' onClick={onReset}>
            {t('Reset filters')}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className='grid gap-3 sm:grid-cols-2 xl:grid-cols-3'>
        <div className='flex min-w-0 flex-col gap-1.5'>
          <span className='text-xs font-medium'>{t('Date range')}</span>
          <ToggleGroup
            value={activePreset === 'custom' ? [] : [activePreset]}
            onValueChange={(presets) => presets[0] && changePreset(presets[0])}
            variant='outline'
            size='sm'
            className='w-full'
          >
            <ToggleGroupItem className='flex-1' value='7'>
              {t('7 days')}
            </ToggleGroupItem>
            <ToggleGroupItem className='flex-1' value='30'>
              {t('30 days')}
            </ToggleGroupItem>
            <ToggleGroupItem className='flex-1' value='90'>
              {t('90 days')}
            </ToggleGroupItem>
          </ToggleGroup>
        </div>
        <div className='flex min-w-0 flex-col gap-1.5'>
          <span className='text-xs font-medium'>{t('Start date')}</span>
          <div>
            <DatePicker
              selected={dayjs(value.startDate).toDate()}
              onSelect={(date) => changeDate('startDate', date)}
            />
          </div>
        </div>
        <div className='flex min-w-0 flex-col gap-1.5'>
          <span className='text-xs font-medium'>{t('End date')}</span>
          <div>
            <DatePicker
              selected={dayjs(value.endDate).toDate()}
              onSelect={(date) => changeDate('endDate', date)}
            />
          </div>
        </div>
        <div className='flex min-w-0 flex-col gap-1.5'>
          <label className='text-xs font-medium' htmlFor='reconciliation-user'>
            {t('User')}
          </label>
          <AsyncSearchCombobox
            key={`user-${selectedUserLabel ?? 'all'}`}
            id='reconciliation-user'
            value={value.userId ? String(value.userId) : undefined}
            selectedLabel={selectedUserLabel}
            options={userOptions}
            isLoading={usersQuery.isFetching}
            placeholder={t('All users')}
            emptyText={t('No matching users')}
            onSearchChange={setUserSearch}
            onValueChange={(next) =>
              onChange({ userId: next ? Number(next) : undefined })
            }
          />
        </div>
        <div className='flex min-w-0 flex-col gap-1.5'>
          <label className='text-xs font-medium' htmlFor='reconciliation-model'>
            {t('Model')}
          </label>
          <AsyncSearchCombobox
            key={`model-${value.modelName ?? 'all'}`}
            id='reconciliation-model'
            value={value.modelName}
            selectedLabel={value.modelName}
            options={modelOptions}
            isLoading={modelsQuery.isFetching}
            placeholder={t('All models')}
            emptyText={t('No matching models')}
            onSearchChange={setModelSearch}
            onValueChange={(next) => onChange({ modelName: next })}
          />
        </div>
        <div className='flex min-w-0 flex-col gap-1.5'>
          <span className='text-xs font-medium'>{t('Granularity')}</span>
          <Select
            items={granularityItems}
            value={value.granularity}
            onValueChange={(next) =>
              onChange({ granularity: next as ReconciliationGranularity })
            }
          >
            <SelectTrigger className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {granularityItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
      </CardContent>
    </Card>
  )
}

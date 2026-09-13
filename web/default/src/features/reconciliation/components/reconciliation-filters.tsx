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
import { useMemo, useState, type ReactNode } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useDebounce } from '@/hooks'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getUserAvatarFallback, getUserAvatarStyle } from '@/lib/avatar'
import dayjs from '@/lib/dayjs'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
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
import {
  getReconciliationModelOptions,
  getReconciliationUserOptions,
} from '../api'
import type {
  ReconciliationFilters as FilterValues,
  ReconciliationGranularity,
} from '../types'

type RangePreset = '7' | '30' | '90' | 'custom'

interface SearchOption {
  label: string
  value: string
  avatarName?: string
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
  onReachEnd?: () => void
  renderOption?: (option: SearchOption) => ReactNode
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
  onReachEnd,
  renderOption,
}: AsyncSearchComboboxProps) {
  const optionMap = useMemo(
    () => new Map(options.map((option) => [option.value, option])),
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
      itemToStringLabel={(item) =>
        optionMap.get(item)?.label ?? selectedLabel ?? item
      }
      onInputValueChange={(next) => {
        setInputValue(next)
        onSearchChange(next)
      }}
      onValueChange={(next) => {
        const selected = next ?? undefined
        setInputValue(
          selected ? (optionMap.get(selected)?.label ?? selected) : ''
        )
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
        <ComboboxList
          onScroll={(event) => {
            const list = event.currentTarget
            if (list.scrollHeight - list.scrollTop - list.clientHeight <= 32) {
              onReachEnd?.()
            }
          }}
        >
          {(item: string) => {
            const option = optionMap.get(item) ?? {
              value: item,
              label: selectedLabel ?? item,
            }
            return (
              <ComboboxItem key={item} value={item}>
                {renderOption ? renderOption(option) : option.label}
              </ComboboxItem>
            )
          }}
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

  const usersQuery = useInfiniteQuery({
    queryKey: ['reconciliation-user-options', debouncedUserSearch],
    initialPageParam: 1,
    queryFn: ({ pageParam }) =>
      getReconciliationUserOptions({
        keyword: debouncedUserSearch.trim(),
        page: pageParam,
        pageSize: 20,
      }),
    getNextPageParam: (lastPage) => {
      const data = lastPage.data
      if (!data || data.page * data.page_size >= data.total) return undefined
      return data.page + 1
    },
  })
  const selectedUserQuery = useQuery({
    queryKey: ['reconciliation-selected-user', value.userId],
    queryFn: () =>
      getReconciliationUserOptions({
        userId: value.userId,
        page: 1,
        pageSize: 1,
      }),
    enabled: Boolean(value.userId),
  })
  const modelsQuery = useInfiniteQuery({
    queryKey: ['reconciliation-model-options', debouncedModelSearch],
    initialPageParam: 1,
    queryFn: ({ pageParam }) =>
      getReconciliationModelOptions({
        keyword: debouncedModelSearch.trim(),
        page: pageParam,
        pageSize: 20,
      }),
    getNextPageParam: (lastPage) => {
      const data = lastPage.data
      if (!data || data.page * data.page_size >= data.total) return undefined
      return data.page + 1
    },
  })

  const userOptions = useMemo<SearchOption[]>(() => {
    return (usersQuery.data?.pages ?? []).flatMap((page) =>
      (page.data?.items ?? []).map((user) => {
        const avatarName = user.username || `#${user.user_id}`
        return {
          value: String(user.user_id),
          label: user.username
            ? `${user.username} (#${user.user_id})`
            : `#${user.user_id}`,
          avatarName,
        }
      })
    )
  }, [usersQuery.data?.pages])
  const modelOptions = useMemo<SearchOption[]>(() => {
    const names = new Set(
      (modelsQuery.data?.pages ?? []).flatMap((page) =>
        (page.data?.items ?? []).map((model) => model.model_name)
      )
    )
    return Array.from(names).map((modelName) => ({
      value: modelName,
      label: modelName,
    }))
  }, [modelsQuery.data?.pages])
  const selectedUser = selectedUserQuery.data?.data?.items[0]
  const selectedUserLabel = selectedUser
    ? selectedUser.username
      ? `${selectedUser.username} (#${selectedUser.user_id})`
      : `#${selectedUser.user_id}`
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
            onReachEnd={() => {
              if (usersQuery.hasNextPage && !usersQuery.isFetchingNextPage) {
                void usersQuery.fetchNextPage()
              }
            }}
            renderOption={(option) => {
              const avatarName = option.avatarName ?? option.label
              return (
                <>
                  <Avatar size='sm'>
                    <AvatarFallback style={getUserAvatarStyle(avatarName)}>
                      {getUserAvatarFallback(avatarName)}
                    </AvatarFallback>
                  </Avatar>
                  <span className='truncate'>{option.label}</span>
                </>
              )
            }}
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
            onReachEnd={() => {
              if (modelsQuery.hasNextPage && !modelsQuery.isFetchingNextPage) {
                void modelsQuery.fetchNextPage()
              }
            }}
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

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
import { useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  type ColumnDef,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { getUserAvatarFallback, getUserAvatarStyle } from '@/lib/avatar'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { DataTableColumnHeader, DataTablePage } from '@/components/data-table'
import { getReconciliationBills } from '../api'
import type { ReconciliationBill, ReconciliationBillsParams } from '../types'

interface ReconciliationTableProps {
  params: ReconciliationBillsParams
  onPaginationChange: (page: number, pageSize: number) => void
}

export function ReconciliationTable({
  params,
  onPaginationChange,
}: ReconciliationTableProps) {
  const { t } = useTranslation()
  const { data, isLoading, isFetching } = useQuery({
    queryKey: ['reconciliation-bills', params],
    queryFn: async () => {
      const response = await getReconciliationBills(params)
      if (!response.success) {
        throw new Error(response.message || 'Request failed')
      }
      return {
        items: response.data?.items ?? [],
        total: response.data?.total ?? 0,
      }
    },
    placeholderData: (previousData) => previousData,
    retry: false,
  })

  const columns = useMemo<ColumnDef<ReconciliationBill>[]>(
    () => [
      {
        accessorKey: 'username',
        enableSorting: false,
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('User')} />
        ),
        cell: ({ row }) => {
          const name = row.original.username || `#${row.original.user_id}`
          return (
            <div className='flex min-w-0 items-center gap-2'>
              <Avatar className='ring-border/60 size-7 shrink-0 ring-1'>
                <AvatarFallback
                  className='text-[11px] font-semibold'
                  style={getUserAvatarStyle(name)}
                >
                  {getUserAvatarFallback(name)}
                </AvatarFallback>
              </Avatar>
              <div className='min-w-0'>
                <div className='truncate text-sm font-medium'>{name}</div>
                <div className='text-muted-foreground text-xs'>
                  #{row.original.user_id}
                </div>
              </div>
            </div>
          )
        },
        meta: { label: t('User'), mobileTitle: true },
      },
      {
        accessorKey: 'period',
        enableSorting: false,
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Period')} />
        ),
        meta: { label: t('Period') },
      },
      {
        accessorKey: 'model_name',
        enableSorting: false,
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Model')} />
        ),
        cell: ({ row }) => (
          <span className='block max-w-80 truncate font-mono text-xs'>
            {row.original.model_name || '-'}
          </span>
        ),
        meta: { label: t('Model') },
      },
      {
        accessorKey: 'amount_usd',
        enableSorting: false,
        header: ({ column }) => (
          <DataTableColumnHeader
            column={column}
            title={t('Fee')}
            className='justify-end text-right'
          />
        ),
        cell: ({ row }) => (
          <div className='text-right font-mono font-medium tabular-nums'>
            {formatBillingCurrencyFromUSD(row.original.amount_usd, {
              abbreviate: false,
              digitsLarge: 2,
              digitsSmall: 2,
            })}
          </div>
        ),
        meta: { label: t('Fee'), mobileBadge: true },
      },
    ],
    [t]
  )

  const table = useReactTable({
    data: data?.items ?? [],
    columns,
    state: {
      pagination: {
        pageIndex: params.page - 1,
        pageSize: params.pageSize,
      },
    },
    onPaginationChange: (updater) => {
      const current = {
        pageIndex: params.page - 1,
        pageSize: params.pageSize,
      }
      const next = typeof updater === 'function' ? updater(current) : updater
      onPaginationChange(next.pageIndex + 1, next.pageSize)
    },
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    pageCount: Math.ceil((data?.total ?? 0) / params.pageSize),
  })

  const pageCount = table.getPageCount()
  useEffect(() => {
    if (pageCount > 0 && params.page > pageCount) {
      onPaginationChange(1, params.pageSize)
    }
  }, [onPaginationChange, pageCount, params.page, params.pageSize])

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No reconciliation records')}
      emptyDescription={t('No consumption records match the current filters.')}
      skeletonKeyPrefix='reconciliation-skeleton'
      toolbar={null}
      tableHeaderClassName='bg-muted/30 sticky top-0 z-10'
      mobileProps={{
        getRowKey: (row) =>
          `${row.original.user_id}-${row.original.period}-${row.original.model_name}`,
      }}
    />
  )
}

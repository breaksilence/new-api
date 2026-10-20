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
import { Download04Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { getRouteApi } from '@tanstack/react-router'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import dayjs from '@/lib/dayjs'
import { handleServerError } from '@/lib/handle-server-error'

import { exportReconciliationBills } from './api'
import { ReconciliationFilters } from './components/reconciliation-filters'
import { ReconciliationTable } from './components/reconciliation-table'
import type {
  ReconciliationFilters as FilterValues,
  ReconciliationGranularity,
} from './types'

const route = getRouteApi('/_authenticated/reconciliation/')

function getDefaultDateRange() {
  const end = dayjs().startOf('day')
  return {
    startDate: end.subtract(29, 'day').format('YYYY-MM-DD'),
    endDate: end.format('YYYY-MM-DD'),
  }
}

export function Reconciliation() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const [isExporting, setIsExporting] = useState(false)
  const defaultRange = useMemo(() => getDefaultDateRange(), [])

  const filters = useMemo<FilterValues>(
    () => ({
      startDate: search.startDate ?? defaultRange.startDate,
      endDate: search.endDate ?? defaultRange.endDate,
      userId: search.userId,
      modelName: search.modelName || undefined,
      granularity: (search.granularity ?? 'day') as ReconciliationGranularity,
    }),
    [defaultRange.endDate, defaultRange.startDate, search]
  )
  const page = search.page ?? 1
  const pageSize = search.pageSize ?? 20

  const updateFilters = useCallback(
    (patch: Partial<FilterValues>) => {
      navigate({
        search: (previous) => ({
          ...previous,
          startDate: patch.startDate ?? previous.startDate,
          endDate: patch.endDate ?? previous.endDate,
          userId: 'userId' in patch ? patch.userId : previous.userId,
          modelName:
            'modelName' in patch ? patch.modelName : previous.modelName,
          granularity: patch.granularity ?? previous.granularity,
          page: undefined,
        }),
      })
    },
    [navigate]
  )
  const resetFilters = useCallback(() => {
    navigate({
      search: (previous) => ({
        ...previous,
        startDate: undefined,
        endDate: undefined,
        userId: undefined,
        modelName: undefined,
        granularity: undefined,
        page: undefined,
      }),
    })
  }, [navigate])
  const updatePagination = useCallback(
    (nextPage: number, nextPageSize: number) => {
      navigate({
        search: (previous) => ({
          ...previous,
          page: nextPage <= 1 ? undefined : nextPage,
          pageSize: nextPageSize,
        }),
      })
    },
    [navigate]
  )

  const handleExport = async () => {
    setIsExporting(true)
    try {
      const blob = await exportReconciliationBills(filters)
      const downloadUrl = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = downloadUrl
      anchor.download = `对账单_${dayjs().format('YYYY-MM-DD')}.xlsx`
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      URL.revokeObjectURL(downloadUrl)
    } catch (error) {
      handleServerError(error, t('Failed to export reconciliation bills'))
    } finally {
      setIsExporting(false)
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Reconciliation')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button onClick={handleExport} disabled={isExporting}>
          {isExporting ? (
            <Spinner data-icon='inline-start' />
          ) : (
            <HugeiconsIcon
              icon={Download04Icon}
              strokeWidth={2}
              data-icon='inline-start'
            />
          )}
          {isExporting ? t('Exporting...') : t('Export')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-3'>
          <ReconciliationFilters
            value={filters}
            onChange={updateFilters}
            onReset={resetFilters}
          />
          <ReconciliationTable
            key={JSON.stringify(filters)}
            params={{ ...filters, page, pageSize }}
            onPaginationChange={updatePagination}
          />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

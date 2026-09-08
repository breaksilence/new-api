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
import { api } from '@/lib/api'
import type {
  ReconciliationBillsParams,
  ReconciliationBillsResponse,
  ReconciliationFilters,
} from './types'

function toApiParams(filters: ReconciliationFilters) {
  return {
    start_date: filters.startDate,
    end_date: filters.endDate,
    user_id: filters.userId,
    model_name: filters.modelName,
    granularity: filters.granularity,
  }
}

export async function getReconciliationBills(
  params: ReconciliationBillsParams
): Promise<ReconciliationBillsResponse> {
  const response = await api.get('/api/reconciliation/bills', {
    params: {
      ...toApiParams(params),
      p: params.page,
      page_size: params.pageSize,
    },
  })
  return response.data
}

export async function exportReconciliationBills(
  filters: ReconciliationFilters
): Promise<Blob> {
  const response = await api.get<Blob>('/api/reconciliation/export', {
    params: toApiParams(filters),
    responseType: 'blob',
    skipBusinessError: true,
    skipErrorHandler: true,
    disableDuplicate: true,
  })

  const contentType = String(response.headers['content-type'] ?? '')
  if (contentType.includes('application/json')) {
    const payload = JSON.parse(await response.data.text()) as {
      success?: boolean
      message?: string
    }
    throw new Error(payload.message || 'Failed to export reconciliation bills')
  }
  return response.data
}

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
export type ReconciliationGranularity = 'day' | 'month'

export interface ReconciliationFilters {
  startDate: string
  endDate: string
  userId?: number
  modelName?: string
  granularity: ReconciliationGranularity
}

export interface ReconciliationBillsParams extends ReconciliationFilters {
  page: number
  pageSize: number
}

export interface ReconciliationBill {
  user_id: number
  username: string
  period: string
  model_name: string
  quota: number
  amount_usd: number
}

export interface ReconciliationBillsResponse {
  success: boolean
  message?: string
  data?: {
    page: number
    page_size: number
    total: number
    items: ReconciliationBill[]
  }
}

export interface ReconciliationOptionSearchParams {
  keyword?: string
  page?: number
  pageSize?: number
}

export interface ReconciliationUserOptionSearchParams extends ReconciliationOptionSearchParams {
  userId?: number
}

export interface ReconciliationUserOption {
  user_id: number
  username: string
}

export interface ReconciliationModelOption {
  model_name: string
}

export interface ReconciliationOptionsResponse<T> {
  success: boolean
  message?: string
  data?: {
    page: number
    page_size: number
    total: number
    items: T[]
  }
}

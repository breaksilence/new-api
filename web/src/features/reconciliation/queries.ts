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
import { queryOptions } from '@tanstack/react-query'

import { requireServerSuccess } from '@/lib/server-error-message'

import type {
  ReconciliationBillsParams,
  ReconciliationBillsResponse,
} from './types'

export function reconciliationBillsQueryOptions(
  params: ReconciliationBillsParams,
  fetchBills: (
    params: ReconciliationBillsParams
  ) => Promise<ReconciliationBillsResponse>
) {
  return queryOptions({
    queryKey: ['reconciliation-bills', params],
    queryFn: async () => {
      const response = await fetchBills(params)
      requireServerSuccess(response)
      return {
        items: response.data?.items ?? [],
        total: response.data?.total ?? 0,
        totalAmountUSD: response.data?.total_amount_usd ?? 0,
      }
    },
    placeholderData: (previousData) => previousData,
    // refetchOnMount alone does not cover switching to a fresh cached page.
    // That page must also fetch so a new filter scope can obtain its total.
    staleTime: 0,
    refetchOnMount: 'always',
    // A filter scope retains its total during pagination. Do not silently
    // refresh only its rows when focus or connectivity changes.
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    retry: false,
  })
}

import { queryOptions } from '@tanstack/react-query'
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
      if (!response.success) {
        throw new Error(response.message || 'Request failed')
      }
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

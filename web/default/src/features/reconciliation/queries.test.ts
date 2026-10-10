import {
  focusManager,
  onlineManager,
  QueryClient,
  QueryObserver,
} from '@tanstack/react-query'
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { reconciliationBillsQueryOptions } from './queries'
import type {
  ReconciliationBillsParams,
  ReconciliationBillsResponse,
} from './types'

test('cached page switching still fetches a fresh total after filters remount', async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { staleTime: 10_000, gcTime: Infinity } },
  })
  const pending = new Map<
    number,
    (value: ReconciliationBillsResponse) => void
  >()
  const options = (page: number) =>
    reconciliationBillsQueryOptions(
      {
        startDate: '2026-09-01',
        endDate: '2026-09-30',
        granularity: 'day',
        page,
        pageSize: 20,
      },
      (params: ReconciliationBillsParams) =>
        new Promise((resolve) => {
          pending.set(params.page, resolve)
        })
    )
  for (const page of [1, 2]) {
    client.setQueryData(options(page).queryKey, {
      items: [],
      total: 40,
      totalAmountUSD: 10,
    })
  }
  const response = (amount: number): ReconciliationBillsResponse => ({
    success: true,
    data: {
      items: [],
      page: 1,
      page_size: 20,
      total: 40,
      total_quota: amount * 500000,
      total_amount_usd: amount,
    },
  })
  const observer = new QueryObserver(client, options(1))
  let displayedTotal: number | null = null
  const consume = () => {
    const result = observer.getCurrentResult()
    if (
      displayedTotal === null &&
      result.isSuccess &&
      result.isFetchedAfterMount &&
      !result.isFetching
    ) {
      displayedTotal = result.data.totalAmountUSD
    }
  }
  const unsubscribe = observer.subscribe(consume)
  try {
    consume()
    assert.equal(
      displayedTotal,
      null,
      'cached total must not flash after filters change'
    )
    observer.setOptions(options(2))
    consume()
    assert.equal(
      pending.size,
      2,
      'cached page 2 must fetch, even within global staleTime'
    )
    assert.equal(displayedTotal, null)
    pending.get(1)!(response(11))
    await new Promise((resolve) => setTimeout(resolve, 0))
    assert.equal(
      displayedTotal,
      null,
      'an obsolete page response must not supply the total'
    )
    pending.get(2)!(response(12))
    await new Promise((resolve) => setTimeout(resolve, 0))
    assert.equal(displayedTotal, 12)
    observer.setOptions(options(3))
    consume()
    assert.equal(
      displayedTotal,
      12,
      'pagination alone must not blank an established total'
    )
    pending.get(3)!(response(13))
    await new Promise((resolve) => setTimeout(resolve, 0))
    assert.equal(
      displayedTotal,
      12,
      'pagination alone must not replace the established total'
    )
  } finally {
    unsubscribe()
    client.clear()
  }
})

test('focus and reconnect do not silently replace rows under a retained total', async () => {
  const client = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        refetchOnWindowFocus: true,
        refetchOnReconnect: true,
      },
    },
  })
  client.mount()
  let requests = 0
  const observer = new QueryObserver(
    client,
    reconciliationBillsQueryOptions(
      {
        startDate: '2026-09-01',
        endDate: '2026-09-30',
        granularity: 'day',
        page: 1,
        pageSize: 20,
      },
      async () => {
        requests++
        return {
          success: true,
          data: {
            items: [],
            page: 1,
            page_size: 20,
            total: requests,
            total_quota: requests * 500000,
            total_amount_usd: requests,
          },
        }
      }
    )
  )
  const unsubscribe = observer.subscribe(() => {})
  try {
    await new Promise((resolve) => setTimeout(resolve, 0))
    assert.equal(requests, 1)
    focusManager.setFocused(false)
    focusManager.setFocused(true)
    onlineManager.setOnline(false)
    onlineManager.setOnline(true)
    await new Promise((resolve) => setTimeout(resolve, 0))
    assert.equal(requests, 1)
    assert.equal(observer.getCurrentResult().data?.totalAmountUSD, 1)
  } finally {
    unsubscribe()
    client.unmount()
    client.clear()
    focusManager.setFocused(undefined)
    onlineManager.setOnline(true)
  }
})

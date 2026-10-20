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
import assert from 'node:assert/strict'

import {
  focusManager,
  onlineManager,
  QueryClient,
  QueryObserver,
} from '@tanstack/react-query'
import { test, vi } from 'vitest'

import { reconciliationBillsQueryOptions } from '../queries'
import type {
  ReconciliationBillsParams,
  ReconciliationBillsResponse,
} from '../types'

test('cached page switching still fetches a fresh total after filters remount', async () => {
  vi.useFakeTimers()
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
    const firstPage = pending.get(1)
    assert.ok(firstPage)
    firstPage(response(11))
    await vi.runAllTimersAsync()
    assert.equal(
      displayedTotal,
      null,
      'an obsolete page response must not supply the total'
    )
    const secondPage = pending.get(2)
    assert.ok(secondPage)
    secondPage(response(12))
    await vi.runAllTimersAsync()
    assert.equal(displayedTotal, 12)
    observer.setOptions(options(3))
    consume()
    assert.equal(
      displayedTotal,
      12,
      'pagination alone must not blank an established total'
    )
    const thirdPage = pending.get(3)
    assert.ok(thirdPage)
    thirdPage(response(13))
    await vi.runAllTimersAsync()
    assert.equal(
      displayedTotal,
      12,
      'pagination alone must not replace the established total'
    )
  } finally {
    unsubscribe()
    client.clear()
    vi.useRealTimers()
  }
})

test('focus and reconnect do not silently replace rows under a retained total', async () => {
  vi.useFakeTimers()
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
    await vi.runAllTimersAsync()
    assert.equal(requests, 1)
    focusManager.setFocused(false)
    focusManager.setFocused(true)
    onlineManager.setOnline(false)
    onlineManager.setOnline(true)
    await vi.runAllTimersAsync()
    assert.equal(requests, 1)
    assert.equal(observer.getCurrentResult().data?.totalAmountUSD, 1)
  } finally {
    unsubscribe()
    client.unmount()
    client.clear()
    focusManager.setFocused(undefined)
    onlineManager.setOnline(true)
    vi.useRealTimers()
  }
})

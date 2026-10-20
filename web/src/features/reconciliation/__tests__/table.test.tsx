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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { SectionPageLayout } from '@/components/layout'
import { api } from '@/lib/api'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { ReconciliationTable } from '../components/reconciliation-table'
import type { ReconciliationBillsResponse } from '../types'

const clients: QueryClient[] = []
afterEach(() => {
  cleanup()
  for (const client of clients.splice(0)) client.clear()
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
})

test('pagination changes rows, retains the filtered total, and shows the full result count', async () => {
  const pending: Array<(value: { data: ReconciliationBillsResponse }) => void> =
    []
  vi.spyOn(api, 'get').mockImplementation(
    () => new Promise((resolve) => pending.push(resolve))
  )
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'USD' },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const filters = {
    startDate: '2026-09-01',
    endDate: '2026-09-30',
    granularity: 'day' as const,
  }
  function Page(props: { userId?: number }) {
    const [page, setPage] = useState(1)
    return (
      <QueryClientProvider client={client}>
        <SectionPageLayout>
          <SectionPageLayout.Content>
            <ReconciliationTable
              key={props.userId ?? 'all'}
              params={{ ...filters, userId: props.userId, page, pageSize: 20 }}
              onPaginationChange={setPage}
            />
          </SectionPageLayout.Content>
        </SectionPageLayout>
      </QueryClientProvider>
    )
  }
  const response = (
    name: string,
    amount: number
  ): { data: ReconciliationBillsResponse } => ({
    data: {
      success: true,
      data: {
        page: 1,
        page_size: 20,
        total: 31,
        total_quota: amount * 500000,
        total_amount_usd: amount,
        items: [
          {
            user_id: 1,
            username: name,
            period: '2026-09-01',
            model_name: 'model-a',
            quota: 500000,
            amount_usd: 1,
          },
        ],
      },
    },
  })
  const resolveNext = async (name: string, amount: number) => {
    await waitFor(() => expect(pending).toHaveLength(1))
    const resolve = pending.shift()
    expect(resolve).toBeDefined()
    await act(async () => resolve?.(response(name, amount)))
  }
  const view = render(<Page />)
  expect(screen.getByText(/Filtered total/)).not.toHaveTextContent('1,234')
  await resolveNext('alice', 1234)
  expect(await screen.findByText('alice')).toBeVisible()
  expect(screen.getByText(/Filtered total/)).toHaveTextContent('1,234.00')
  expect(screen.getByText('31')).toBeVisible()
  await userEvent.click(screen.getByRole('button', { name: 'Go to next page' }))
  await waitFor(() =>
    expect(api.get).toHaveBeenLastCalledWith(
      '/api/reconciliation/bills',
      expect.objectContaining({ params: expect.objectContaining({ p: 2 }) })
    )
  )
  expect(screen.getByText(/Filtered total/)).toHaveTextContent('1,234.00')
  await resolveNext('bob', 1500)
  expect(await screen.findByText('bob')).toBeVisible()
  expect(screen.queryByText('alice')).not.toBeInTheDocument()
  expect(screen.getByText(/Filtered total/)).toHaveTextContent('1,234.00')
  view.rerender(<Page userId={2} />)
  expect(screen.getByText(/Filtered total/)).not.toHaveTextContent('1,234')
  await resolveNext('bob-filtered', 2000)
  expect(await screen.findByText('bob-filtered')).toBeVisible()
  expect(screen.getByText(/Filtered total/)).toHaveTextContent('2,000.00')
})

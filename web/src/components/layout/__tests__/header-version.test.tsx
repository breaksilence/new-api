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
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { SidebarProvider } from '@/components/ui/sidebar'
import { useSystemUpdateStore } from '@/features/system-update/store'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { STATUS_QUERY_KEY } from '@/lib/status-query'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { AppHeader } from '../components/app-header'
import { PublicHeader } from '../components/public-header'

let client: QueryClient

beforeEach(() => {
  localStorage.clear()
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'admin',
    role: ROLE.SUPER_ADMIN,
  })
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  useSystemConfigStore.getState().setLoading(false)
  useSystemUpdateStore.setState({ snapshot: null })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response('[]'))
  )
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  useSystemUpdateStore.setState({ snapshot: null })
  localStorage.clear()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

test.each([
  ['console', 'v1.0.0-rc.45-lx'],
  ['console', ''],
  ['public', 'v1.0.0-rc.45-lx'],
  ['public', ''],
])(
  '%s header hides the version entry for an admin with version "%s"',
  async (kind, version) => {
    const status = { version, system_name: 'New API' }
    client.setQueryData(STATUS_QUERY_KEY, status)
    vi.spyOn(api, 'get').mockImplementation(async (url) => ({
      data: { success: true, data: url === '/api/notice' ? '' : status },
    }))
    const header =
      kind === 'console' ? (
        <SidebarProvider>
          <AppHeader rightContent={false} />
        </SidebarProvider>
      ) : (
        <PublicHeader
          showThemeSwitch={false}
          showLanguageSwitcher={false}
          showAuthButtons={false}
          showNotifications={false}
        />
      )
    const router = createRouter({
      routeTree: createRootRoute({ component: () => header }),
      history: createMemoryHistory({ initialEntries: ['/'] }),
    })
    await router.load()
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    )

    const banner = within(await screen.findByRole('banner'))
    expect(banner.getByText('New API')).toBeVisible()
    expect(banner.getByRole('img', { name: /logo/i })).toBeInTheDocument()
    expect(
      banner.queryByRole('button', { name: /System updates, current version:/ })
    ).not.toBeInTheDocument()
    expect(
      banner.queryByText(version || 'Unknown version')
    ).not.toBeInTheDocument()
  }
)

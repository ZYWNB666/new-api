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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { OverviewDashboard } from '../overview-dashboard'

let client: QueryClient
let keyLookupError: Error | null

beforeEach(() => {
  window.localStorage.clear()
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'dashboard-user',
    role: 1,
    quota: 1000000,
    used_quota: 1000,
    request_count: 1,
  })
  client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  keyLookupError = null
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    switch (url) {
      case '/api/token/?p=1&size=10':
        if (keyLookupError) throw keyLookupError
        return {
          data: {
            success: true,
            data: {
              items: [{ id: 1, name: 'App key', key: 'masked', status: 1 }],
            },
          },
        }
      case '/api/status':
        return {
          data: {
            data: {
              api_info_enabled: false,
              announcements_enabled: false,
              faq_enabled: false,
              uptime_kuma_enabled: false,
            },
          },
        }
      case '/api/user/models':
        return { data: { success: true, data: ['gpt-4o-mini'] } }
      case '/api/data/self':
        return { data: { success: true, data: [] } }
      case '/api/perf-metrics/summary':
        return { data: { success: true, data: { summary: null, models: [] } } }
      default:
        if (url.startsWith('/api/log/self')) {
          return {
            data: {
              success: true,
              data: { items: [], total: 0, page: 1, page_size: 5 },
            },
          }
        }
        if (url.startsWith('/api/perf-metrics/summary')) {
          return { data: { success: true, data: { summary: null, models: [] } } }
        }
        throw new Error(`Unexpected dashboard request: ${url}`)
    }
  })
})

afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  window.localStorage.clear()
})

async function renderOverview() {
  const router = createRouter({
    routeTree: createRootRoute({ component: OverviewDashboard }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('overview setup guide', () => {
  it('shows usage first and only a header entry when setup is complete', async () => {
    await renderOverview()

    const toggle = await screen.findByRole('button', { name: 'Setup guide' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    const headings = screen
      .getAllByRole('heading')
      .map((heading) => heading.textContent)
    expect(headings[0]).toBe('Overview')
    expect(headings).toContain('Usage trend')
    expect(headings).toContain('Quick actions')
    expect(screen.queryByText('Setup guide complete')).not.toBeInTheDocument()
    expect(screen.queryByText('Setup progress: 3/3')).not.toBeInTheDocument()
    for (const name of ['API Keys', 'Channels', 'Usage Logs', 'Pricing']) {
      expect(screen.queryByRole('button', { name })).not.toBeInTheDocument()
    }
    const panel = document.querySelector(
      `[id="${toggle.getAttribute('aria-controls') ?? ''}"]`
    )
    expect(panel).toBeInTheDocument()
    expect(panel).not.toBeVisible()
  })

  it('toggles the completed guide with the keyboard and restores focus after hiding it', async () => {
    const user = userEvent.setup()
    await renderOverview()
    const toggle = await screen.findByRole('button', { name: 'Setup guide' })

    await user.tab()
    expect(toggle).toHaveFocus()
    await user.keyboard('{Enter}')
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(
      document.querySelector(
        `[id="${toggle.getAttribute('aria-controls') ?? ''}"]`
      )
    ).toBeVisible()
    expect(
      screen.getByRole('heading', {
        name: 'Build on your API gateway in minutes',
      })
    ).toBeVisible()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /^API Keys/ })).toBeVisible()
    )

    await user.keyboard(' ')
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(toggle).toHaveFocus()
    await user.click(toggle)
    await user.click(screen.getByRole('button', { name: 'Hide setup guide' }))
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(toggle).toHaveFocus()
  })

  it('restores the completed guide preference after remounting', async () => {
    const user = userEvent.setup()
    const first = await renderOverview()
    await user.click(await screen.findByRole('button', { name: 'Setup guide' }))
    first.unmount()

    const second = await renderOverview()
    expect(
      await screen.findByRole('button', { name: 'Setup guide' })
    ).toHaveAttribute('aria-expanded', 'true')
    await user.click(screen.getByRole('button', { name: 'Hide setup guide' }))
    second.unmount()

    await renderOverview()
    expect(
      await screen.findByRole('button', { name: 'Setup guide' })
    ).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('Setup guide complete')).not.toBeInTheDocument()
  })

  it('展开入门引导后概览指标仍在上，并保留充值入口且不泄露管理员渠道', async () => {
    const user = userEvent.setup()
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...useSystemConfigStore.getState().config.currency,
        quotaDisplayType: 'CUSTOM',
        customCurrencySymbol: 'LONG-CURRENCY-SYMBOL',
      },
    })
    client.setQueryData(['status'], {
      quota_display_type: 'CUSTOM',
      custom_currency_symbol: 'LONG-CURRENCY-SYMBOL',
    })
    await renderOverview()
    await user.click(await screen.findByRole('button', { name: 'Setup guide' }))
    const metrics = screen.getByText('Account balance')
    const guide = screen.getByRole('heading', {
      name: 'Build on your API gateway in minutes',
    })
    expect(
      metrics.compareDocumentPosition(guide) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(screen.getByText('LONG-CURRENCY-SYMBOL 2')).toBeVisible()
    expect(
      screen.getByRole('link', { name: /Top up/ })
    ).toHaveAttribute('href', '/wallet')
    expect(
      screen.queryByRole('button', { name: /^Channels/ })
    ).not.toBeInTheDocument()
  })

  it('管理员展开引导时保留渠道入口与概览指标卡', async () => {
    const user = userEvent.setup()
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'dashboard-admin',
      role: 10,
      quota: 1000000,
      used_quota: 1000,
      request_count: 123,
    })
    await renderOverview()
    await user.click(await screen.findByRole('button', { name: 'Setup guide' }))
    expect(
      await screen.findByRole('button', { name: /^Channels/ })
    ).toHaveAttribute('href', '/channels')
    expect(screen.getByText('Account balance')).toBeVisible()
    expect(screen.getByText("Today's requests")).toBeVisible()
  })

  it('keeps the overview focused and the guide collapsed by default even if key lookup fails', async () => {
    keyLookupError = new Error('Key lookup unavailable')
    await renderOverview()

    // The overview mirrors the reference dashboard: the setup guide stays
    // collapsed by default and never auto-expands into the metrics area.
    expect(screen.queryByText(/Setup progress:/)).not.toBeInTheDocument()
    expect(screen.getByText('Account balance')).toBeVisible()
    expect(screen.getByText('Usage trend')).toBeVisible()
  })
})

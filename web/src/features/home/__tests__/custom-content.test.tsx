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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { Home } from '../index'

let client: QueryClient
let content: string
beforeEach(() => {
  window.localStorage.clear()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  useSystemConfigStore.getState().setLoading(false)
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(['status'], {
    HeaderNavModules: { home: true, about: true },
  })
  client.setQueryData(['notice'], { success: true, data: '' })
  content = ''
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url !== '/api/home_page_content') {
      throw new Error(`Unexpected public request: ${url}`)
    }
    return { data: { success: true, data: content } }
  })
})
afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  window.localStorage.clear()
})
async function renderHome() {
  const router = createRouter({
    routeTree: createRootRoute({ component: Home }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('custom home content', () => {
  it('管理员 Markdown 优先于默认 Hero 和 CTA，保留公开导航', async () => {
    content = '# Custom welcome\n\nAdministrator content.'
    await renderHome()
    expect(
      await screen.findByRole('heading', { name: 'Custom welcome' })
    ).toBeVisible()
    expect(screen.queryByText('Your applications')).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Documentation' })).toHaveAttribute(
      'href',
      '/docs'
    )
  })

  it('管理员 HTML 使用隔离渲染，页面样式不能覆盖公开导航', async () => {
    content = '<style>h1 { color: red; }</style><h1>Isolated welcome</h1>'
    const view = await renderHome()
    await waitFor(() => {
      const host = view.container.querySelector('.custom-home-content')
      expect(host?.shadowRoot?.textContent).toContain('Isolated welcome')
    })
    expect(
      screen.queryByRole('heading', { name: 'Isolated welcome' })
    ).not.toBeInTheDocument()
    expect(screen.queryByText('Your applications')).not.toBeInTheDocument()
  })

  it('管理员 URL 保留受限 iframe 和既有用户激活跳转，不添加同源权限', async () => {
    content = 'https://example.com/custom-home'
    await renderHome()
    const frame = await screen.findByTitle('Custom Home Page')
    expect(frame).toHaveAttribute('src', content)
    expect(frame).toHaveAttribute(
      'sandbox',
      'allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts allow-top-navigation-by-user-activation'
    )
    expect(screen.queryByText('Your applications')).not.toBeInTheDocument()
  })
})

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

import { About } from '../index'

let client: QueryClient
let content: string
beforeEach(() => {
  window.localStorage.clear()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  useSystemConfigStore
    .getState()
    .setConfig({ systemName: 'Configured gateway' })
  useSystemConfigStore.getState().setLoading(false)
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  client.setQueryData(['status'], {
    HeaderNavModules: { home: true, about: true },
  })
  client.setQueryData(['notice'], { success: true, data: '' })
  content = ''
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url !== '/api/about') {
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
async function renderAbout() {
  const router = createRouter({
    routeTree: createRootRoute({ component: About }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('about content', () => {
  it('未配置内容时使用管理员系统名称并保留真实项目归属', async () => {
    await renderAbout()
    expect(
      await screen.findByRole('heading', {
        name: 'About Configured gateway',
        level: 1,
      })
    ).toBeVisible()
    expect(
      screen.getByText(
        'Built by the New API community and QuantumNous, the platform provides AI model API access, token management, quota management, model information and call records.'
      )
    ).toBeVisible()
    expect(
      screen.getByRole('link', { name: /Go to Console/ })
    ).toHaveAttribute('href', '/dashboard')
  })

  it('管理员 Markdown 优先于默认介绍', async () => {
    content = '# Custom about\n\nAdministrator description.'
    await renderAbout()
    expect(
      await screen.findByRole('heading', { name: 'Custom about' })
    ).toBeVisible()
    expect(
      screen.queryByRole('heading', { name: 'About Configured gateway' })
    ).not.toBeInTheDocument()
  })

  it('管理员 HTML 保持隔离渲染，不混入默认介绍', async () => {
    content = '<style>h1 { color: red; }</style><h1>Isolated about</h1>'
    const view = await renderAbout()
    await waitFor(() => {
      const host = view.container.querySelector('.prose-neutral')
      expect(host?.shadowRoot?.textContent).toContain('Isolated about')
    })
    expect(
      screen.queryByRole('heading', { name: 'Configured gateway' })
    ).not.toBeInTheDocument()
  })

  it('管理员 URL 保留 iframe sandbox，不添加同源访问权限', async () => {
    content = 'https://example.com/custom-about'
    const view = await renderAbout()
    const frame = await waitFor(() => {
      const frame = view.container.querySelector('iframe[title="About"]')
      if (!frame) {
        throw new Error('Missing custom About frame')
      }
      return frame
    })
    expect(frame).toHaveAttribute('src', content)
    expect(frame).toHaveAttribute(
      'sandbox',
      'allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts'
    )
    expect(
      screen.queryByRole('heading', { name: 'About Configured gateway' })
    ).not.toBeInTheDocument()
  })
})

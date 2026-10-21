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
import {
  act,
  cleanup,
  render,
  renderHook,
  screen,
} from '@testing-library/react'
import i18next from 'i18next'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { PublicHeader } from '@/components/layout/components/public-header'
import zh from '@/i18n/locales/zh.json'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { useTopNavLinks } from '../use-top-nav-links'

let client: QueryClient
const missingKey = vi.fn()
const saveMissing = i18next.options.saveMissing
beforeEach(() => {
  window.localStorage.clear()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  client = new QueryClient()
  client.setQueryData(['status'], {
    docs_link: 'https://docs.example.com',
    HeaderNavModules: {
      home: true,
      console: true,
      docs: true,
      about: true,
      pricing: { enabled: true, requireAuth: true },
      rankings: { enabled: true, requireAuth: true },
    },
  })
})
afterEach(async () => {
  cleanup()
  i18next.off('missingKey', missingKey)
  i18next.options.saveMissing = saveMissing
  missingKey.mockReset()
  await i18next.changeLanguage('en')
  client.clear()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  window.localStorage.clear()
})
function wrapper(props: { children: ReactNode }) {
  return (
    <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
  )
}

async function renderNavigation(navLinks?: { title: string; href: string }[]) {
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => <PublicHeader navLinks={navLinks} />,
    }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  client.setQueryData(['notice'], { success: true, data: '' })
  await router.load()
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('top navigation links', () => {
  it('全部模块关闭时公开导航不恢复默认入口', async () => {
    client.setQueryData(['status'], {
      HeaderNavModules: {
        home: false,
        console: false,
        docs: false,
        about: false,
        pricing: { enabled: false },
        rankings: { enabled: false },
      },
    })
    const view = await renderNavigation()
    expect(view.container.querySelector('a[href="/docs"]')).toBeNull()
    expect(view.container.querySelector('a[href="/pricing"]')).toBeNull()
    expect(view.container.querySelector('a[href="/about"]')).toBeNull()
  })

  it('切换中文后仅翻译导航来源键一次', async () => {
    i18next.addResourceBundle('zhCN', 'translation', zh.translation, true, true)
    await i18next.changeLanguage('zhCN')
    i18next.options.saveMissing = true
    i18next.on('missingKey', missingKey)
    await renderNavigation()
    expect(screen.getAllByRole('link', { name: '文档' })).not.toHaveLength(0)
    expect(missingKey).not.toHaveBeenCalledWith(
      expect.anything(),
      'translation',
      '文档',
      expect.anything()
    )
    i18next.options.saveMissing = false
    await act(() => i18next.changeLanguage('en'))
    expect(
      screen.getAllByRole('link', { name: 'Documentation' })
    ).not.toHaveLength(0)
    expect(screen.queryByRole('link', { name: '文档' })).toBeNull()
  })

  it('启用文档时提供站内入口，同时保留已有公开模块', () => {
    const { result } = renderHook(() => useTopNavLinks(), { wrapper })
    expect(result.current).toContainEqual({
      title: 'Documentation',
      href: '/docs',
    })
    expect(result.current.some((link) => link.href === '/')).toBe(true)
    expect(result.current.some((link) => link.href === '/about')).toBe(true)
  })

  it('管理员关闭导航模块时不显示入口', () => {
    client.setQueryData(['status'], {
      HeaderNavModules: {
        home: false,
        console: false,
        docs: false,
        about: false,
        pricing: { enabled: false },
        rankings: { enabled: false },
      },
    })
    const { result } = renderHook(() => useTopNavLinks(), { wrapper })
    expect(result.current).toEqual([])
  })

  it('未登录的受限模块保持鉴权标记，登录后只移除导航提示而不更改目的地', () => {
    const { result } = renderHook(() => useTopNavLinks(), { wrapper })
    for (const href of ['/pricing', '/rankings']) {
      expect(
        result.current.find((link) => link.href === href)?.requiresAuth
      ).toBe(true)
    }
    act(() =>
      useAuthStore
        .getState()
        .auth.setUser({ id: 1, username: 'navigation-user', role: 1 })
    )
    for (const href of ['/pricing', '/rankings']) {
      expect(
        result.current.find((link) => link.href === href)?.requiresAuth
      ).toBe(false)
    }
    expect(
      result.current.find((link) => link.href === '/docs')?.requiresAuth
    ).toBeUndefined()
  })
})

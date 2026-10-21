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
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ThemeProvider } from '@/context/theme-provider'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { AuthLayout } from '../auth-layout'

let client: QueryClient
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  useSystemConfigStore.getState().setConfig({
    systemName: 'Configured gateway',
    logo: '/configured-logo.svg',
  })
  useSystemConfigStore.getState().setLoadedLogoUrl('/configured-logo.svg')
  useSystemConfigStore.getState().setLoading(false)
})
afterEach(() => {
  cleanup()
  client.clear()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  window.localStorage.clear()
  document.cookie = 'auth-layout-test-theme=; max-age=0; path=/'
  document.documentElement.classList.remove('dark', 'light')
})

async function renderLayout(compact = false) {
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => (
        <AuthLayout compact={compact}>
          <form aria-label='Authentication'>
            <label htmlFor='account'>Account</label>
            <Input id='account' disabled />
            <p>{'Long form content '.repeat(80)}</p>
            <Button disabled aria-busy='true'>
              Continue
            </Button>
          </form>
        </AuthLayout>
      ),
    }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  return render(
    <QueryClientProvider client={client}>
      <ThemeProvider defaultTheme='dark' storageKey='auth-layout-test-theme'>
        <RouterProvider router={router} />
      </ThemeProvider>
    </QueryClientProvider>
  )
}

describe('authentication layout', () => {
  it('长表单保留自然页面滚动、管理员品牌和禁用加载状态，不覆盖已有暗色主题', async () => {
    const view = await renderLayout()
    expect(
      await screen.findByRole('form', { name: 'Authentication' })
    ).toBeVisible()
    const page = view.container.querySelector('.auth-page')
    expect(page).not.toHaveClass('h-svh', 'overflow-hidden')
    expect(screen.getByRole('main')).toHaveClass('lg:grid-cols-[1fr_1fr]')
    expect(screen.getByRole('complementary')).toHaveClass('hidden', 'lg:block')
    expect(screen.getAllByRole('img', { name: 'Logo' })[0]).toHaveAttribute(
      'src',
      '/configured-logo.svg'
    )
    expect(screen.getAllByText('Configured gateway').length).toBeGreaterThan(0)
    expect(screen.getByRole('textbox', { name: 'Account' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Continue' })).toHaveAttribute(
      'aria-busy',
      'true'
    )
    expect(document.documentElement).toHaveClass('dark')
  })

  it('紧凑验证布局省略展示侧栏，仍能通过键盘操作主题切换', async () => {
    const user = userEvent.setup()
    await renderLayout(true)
    expect(
      await screen.findByRole('form', { name: 'Authentication' })
    ).toBeVisible()
    expect(screen.queryByRole('complementary')).not.toBeInTheDocument()
    const toggle = screen.getByRole('button', { name: 'Toggle theme' })
    toggle.focus()
    await user.keyboard('{Enter}')
    await user.click(await screen.findByRole('menuitem', { name: 'Light' }))
    expect(document.documentElement).toHaveClass('light')
    expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled()
  })
})

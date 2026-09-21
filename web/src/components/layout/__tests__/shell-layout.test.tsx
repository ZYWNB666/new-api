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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ComponentProps, ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { AuthenticatedLayout } from '../components/authenticated-layout'
import { PublicHeader } from '../components/public-header'

vi.mock('@tanstack/react-router', () => ({
  Link: (props: ComponentProps<'a'> & { to: string }) => {
    const {
      disabled: _disabled,
      to,
      ...linkProps
    } = props as ComponentProps<'a'> & {
      disabled?: boolean
      to: string
    }
    return <a href={to} {...linkProps} />
  },
  useNavigate: () => vi.fn(),
  useRouterState: () => ({ location: { pathname: '/' } }),
}))

vi.mock('@/components/dialog', () => ({ Dialog: () => null }))
vi.mock('@/components/language-switcher', () => ({
  LanguageSwitcher: () => <button type='button'>Language</button>,
}))
vi.mock('@/components/notification-popover', () => ({
  NotificationPopover: () => <button type='button'>Notifications</button>,
}))
vi.mock('@/components/profile-dropdown', () => ({
  ProfileDropdown: () => <button type='button'>Profile</button>,
}))
vi.mock('@/components/theme-switch', () => ({
  ThemeSwitch: () => <button type='button'>Theme</button>,
}))
vi.mock('@/features/system-update/system-update-action', () => ({
  SystemUpdateAction: () => null,
}))
vi.mock('@/hooks/use-notifications', () => ({
  useNotifications: () => ({
    popoverOpen: false,
    setPopoverOpen: vi.fn(),
    unreadCount: 0,
    activeTab: 'notice',
    setActiveTab: vi.fn(),
    notice: [],
    announcements: [],
    loading: false,
  }),
}))
vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({
    systemName: 'New API',
    logo: '/logo.png',
    loading: false,
    logoLoaded: true,
  }),
}))
vi.mock('@/hooks/use-top-nav-links', () => ({ useTopNavLinks: () => [] }))
vi.mock('@/stores/auth-store', () => ({
  useAuthStore: () => ({ auth: { user: null } }),
}))

vi.mock('@/components/page-transition', () => ({
  AnimatedOutlet: () => <div>Outlet</div>,
}))
vi.mock('@/components/skip-to-main', () => ({ SkipToMain: () => null }))
vi.mock('@/context/layout-provider', () => ({
  LayoutProvider: (props: { children: ReactNode }) => props.children,
}))
vi.mock('@/context/search-provider', () => ({
  SearchProvider: (props: { children: ReactNode }) => props.children,
}))
vi.mock('@/lib/cookies', () => ({ getCookie: () => undefined }))
vi.mock('../components/app-header', () => ({
  AppHeader: () => <header>Application header</header>,
}))
vi.mock('../components/app-sidebar', () => ({ AppSidebar: () => <aside /> }))
vi.mock('@/components/ui/sidebar', () => ({
  SidebarProvider: (props: { children: ReactNode; className?: string }) => (
    <div data-testid='sidebar-provider' className={props.className}>
      {props.children}
    </div>
  ),
  SidebarInset: (props: ComponentProps<'main'>) => <main {...props} />,
}))

describe('reference-informed shell layout', () => {
  it('keeps the public navigation in a capsule before and after scrolling', async () => {
    Object.defineProperty(window, 'scrollY', { configurable: true, value: 0 })
    render(
      <PublicHeader
        navLinks={[{ title: 'Pricing', href: '/pricing' }]}
        showNotifications={false}
      />
    )

    const [navigation] = screen.getAllByRole('navigation')
    expect(navigation).toHaveClass('h-12', 'rounded-full')

    Object.defineProperty(window, 'scrollY', { configurable: true, value: 24 })
    fireEvent.scroll(window)

    await waitFor(() => {
      expect(navigation).toHaveClass('h-12', 'rounded-full')
    })
  })

  it('renders authenticated content in the bordered rounded workspace', () => {
    render(<AuthenticatedLayout>Workspace content</AuthenticatedLayout>)

    expect(screen.getByTestId('sidebar-provider')).toHaveClass(
      'bg-[var(--shell-canvas)]'
    )
    expect(screen.getByRole('main')).toHaveClass(
      'bg-[var(--shell-workspace)]',
      'md:peer-data-[variant=inset]:rounded-[1.5rem]',
      'md:peer-data-[variant=inset]:border'
    )
    expect(screen.getByText('Workspace content')).toBeInTheDocument()
  })
})

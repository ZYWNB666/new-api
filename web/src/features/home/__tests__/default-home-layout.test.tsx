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
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { Home } from '../index'

vi.mock('@/components/layout', () => ({
  PublicLayout: (props: { children: ReactNode }) => (
    <main>{props.children}</main>
  ),
}))
vi.mock('@/context/theme-provider', () => ({
  useTheme: () => ({ resolvedTheme: 'light' }),
}))
vi.mock('@/stores/auth-store', () => ({
  useAuthStore: () => ({ auth: { user: null } }),
}))
vi.mock('../hooks', () => ({
  useHomePageContent: () => ({ content: '', isLoaded: true, isUrl: false }),
}))
vi.mock('../components', () => ({
  Hero: () => <div>Hero section</div>,
  Features: () => <div>Features section</div>,
  HowItWorks: () => <div>How it works section</div>,
  JevShowcase: () => <div>Jev showcase section</div>,
  CTA: () => <div>CTA section</div>,
}))
vi.mock('@/components/layout/components/footer', () => ({
  Footer: () => <footer>Project footer</footer>,
}))

describe('default home layout', () => {
  it('ends after the jev showcase section with the company footer', () => {
    render(<Home />)

    expect(screen.getByText('Hero section')).toBeInTheDocument()
    expect(screen.getByText('Features section')).toBeInTheDocument()
    expect(screen.getByText('How it works section')).toBeInTheDocument()
    expect(screen.getByText('Jev showcase section')).toBeInTheDocument()
    expect(screen.queryByText('CTA section')).not.toBeInTheDocument()
    expect(screen.getByText('Project footer')).toBeInTheDocument()
  })
})

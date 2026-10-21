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
  Stats: () => <div>Stats section</div>,
  ProductShowcase: () => <div>Product showcase section</div>,
  ModelEcosystem: () => <div>Model ecosystem section</div>,
  JevShowcase: () => <div>Jev showcase section</div>,
  CTA: () => <div>CTA section</div>,
}))
vi.mock('@/components/layout/components/footer', () => ({
  Footer: () => <footer>Project footer</footer>,
}))

describe('default home layout', () => {
  it('在 Jev 展示后显示接入操作区，并保留项目页脚', () => {
    render(<Home />)

    expect(screen.getByText('Hero section')).toBeInTheDocument()
    expect(screen.getByText('Stats section')).toBeInTheDocument()
    expect(screen.getByText('Product showcase section')).toBeInTheDocument()
    expect(screen.getByText('Model ecosystem section')).toBeInTheDocument()
    expect(screen.getByText('Jev showcase section')).toBeInTheDocument()
    expect(screen.getByText('CTA section')).toBeInTheDocument()
    expect(
      screen
        .getByText('Jev showcase section')
        .compareDocumentPosition(screen.getByText('CTA section')) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(
      screen
        .getByText('CTA section')
        .compareDocumentPosition(screen.getByText('Project footer')) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(
      screen
        .getByText('Hero section')
        .compareDocumentPosition(screen.getByText('Stats section')) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(
      screen
        .getByText('Stats section')
        .compareDocumentPosition(
          screen.getByText('Product showcase section')
        ) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(
      screen
        .getByText('Product showcase section')
        .compareDocumentPosition(
          screen.getByText('Model ecosystem section')
        ) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(screen.getByText('Project footer')).toBeInTheDocument()
  })
})

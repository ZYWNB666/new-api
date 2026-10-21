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
import type { ComponentProps } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { Hero } from '../components/sections/hero'

vi.mock('@tanstack/react-router', () => ({
  Link: (props: ComponentProps<'a'> & { to: string }) => {
    const { to, ...linkProps } = props
    return <a href={to} {...linkProps} />
  },
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: { docs_link: 'https://docs.example.com' } }),
}))

vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({ systemName: 'Example AI', logo: '/logo.png' }),
}))

describe('home hero', () => {
  it('uses configured branding and presents the supported model catalogue', () => {
    render(<Hero />)

    expect(
      screen.getByRole('heading', {
        name: /Example AI.*Vast Range of AI Models/,
      })
    ).toBeInTheDocument()

    for (const model of [
      'Qwen',
      'DeepSeek',
      'Zhipu GLM',
      'Doubao',
      'Kimi',
      'MiniMax',
      'Baichuan',
      'ERNIE',
    ]) {
      expect(screen.getByText(model)).toBeInTheDocument()
    }
  })

  it('展示可核实能力及连接示意，并提供注册和文档入口', () => {
    render(<Hero />)
    expect(screen.getByText('Your applications')).toBeVisible()
    expect(
      screen.getAllByText('Multi-protocol Compatible').length
    ).toBeGreaterThan(0)
    expect(
      screen.queryByText(/99\.99%|50ms|50\+|100\+/)
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Documentation' })
    ).toHaveAttribute('href', '/docs')
    expect(screen.getByRole('button', { name: /Get Started/ })).toHaveAttribute(
      'href',
      '/sign-up'
    )
    expect(screen.getByText('Qwen').closest('.landing-model-strip')).not.toBeNull()
  })

  it('keeps the authenticated dashboard action', () => {
    render(<Hero isAuthenticated />)

    expect(
      screen.getByRole('button', { name: /Go to Dashboard/ })
    ).toHaveAttribute('href', '/dashboard')
    expect(screen.queryByRole('button', { name: /Get Started/ })).toBeNull()
  })

  it('保留带无障碍隐藏属性的装饰波纹', () => {
    const { container } = render(<Hero />)
    const waves = container.querySelectorAll('.landing-wave')

    expect(waves).toHaveLength(2)
    for (const wave of waves) {
      expect(wave).toHaveAttribute('aria-hidden', 'true')
      expect(wave).toHaveClass('pointer-events-none')
    }
  })
})

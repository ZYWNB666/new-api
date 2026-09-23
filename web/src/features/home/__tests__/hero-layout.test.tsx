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
  useSystemConfig: () => ({ systemName: 'Example AI' }),
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
      'GLM',
      'Doubao',
      'Kimi',
      'Yi',
      'Baichuan',
      'Spark',
    ]) {
      expect(screen.getByText(model)).toBeInTheDocument()
    }
  })

  it('shows capability counts instead of live reliability claims', () => {
    render(<Hero />)

    expect(screen.getAllByText('50+')).toHaveLength(2)
    expect(screen.getByText('100+')).toBeInTheDocument()
    expect(screen.getByText('upstream services integrated')).toBeInTheDocument()
    expect(screen.getByText('model billing support')).toBeInTheDocument()
    expect(screen.getByText('compatible API routes')).toBeInTheDocument()
    expect(screen.queryByText(/99\.99%|50ms/)).not.toBeInTheDocument()
    expect(
      screen.getByText('100+').closest('.landing-metric-card')
    ).not.toBeNull()
    expect(screen.getByText('Qwen').parentElement).toHaveClass(
      'landing-model-strip'
    )
  })

  it('keeps the authenticated dashboard action', () => {
    render(<Hero isAuthenticated />)

    expect(
      screen.getByRole('button', { name: /Go to Dashboard/ })
    ).toHaveAttribute('href', '/dashboard')
    expect(screen.queryByRole('button', { name: /Get Started/ })).toBeNull()
  })

  it('renders decorative green wave layers behind the hero content', () => {
    const { container } = render(<Hero />)
    const waves = container.querySelectorAll('.landing-wave')

    expect(waves).toHaveLength(2)
    for (const wave of waves) {
      expect(wave).toHaveAttribute('aria-hidden', 'true')
      expect(wave).toHaveClass('pointer-events-none')
    }
  })
})

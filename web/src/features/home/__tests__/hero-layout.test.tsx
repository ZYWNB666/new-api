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
import { act, render, screen } from '@testing-library/react'
import type { ComponentProps } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { HeroTerminalDemo } from '../components/hero-terminal-demo'
import { Hero } from '../components/sections/hero'
import { JevShowcase } from '../components/sections/jev-showcase'

const config = vi.hoisted(() => ({ serverAddress: '' }))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: ComponentProps<'a'> & { to: string }) => {
    const { to, ...linkProps } = props
    return <a href={to} {...linkProps} />
  },
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({
    status: { server_address: config.serverAddress },
  }),
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
    ]) {
      expect(screen.getByText(model)).toBeInTheDocument()
    }
  })

  it('展示网关核心视觉与统计卡，并提供注册和文档入口', () => {
    const { container } = render(<Hero />)
    expect(
      screen.getAllByText('Multi-protocol Compatible').length
    ).toBeGreaterThan(0)
    expect(screen.getByText('100+')).toBeInTheDocument()
    expect(screen.getByText('99.9%')).toBeInTheDocument()
    expect(screen.getByText('<100ms')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Documentation' })
    ).toHaveAttribute('href', '/docs')
    expect(screen.getByRole('button', { name: /Get Started/ })).toHaveAttribute(
      'href',
      '/sign-up'
    )
    expect(container.querySelector('.hero-orbit-core')).not.toBeNull()
    expect(
      container.querySelectorAll('.hero-float-card').length
    ).toBeGreaterThan(0)
  })

  it('keeps the gateway caption in normal flow below the clipped artwork', () => {
    render(<Hero />)

    const caption = screen.getByText(
      'One gateway routes your app to leading models'
    )
    expect(caption).not.toHaveClass('absolute')
    expect(caption.parentElement).not.toHaveClass('overflow-hidden')
  })

  it('keeps the authenticated dashboard action', () => {
    render(<Hero isAuthenticated />)

    expect(
      screen.getByRole('button', { name: /Go to Dashboard/ })
    ).toHaveAttribute('href', '/dashboard')
    expect(screen.queryByRole('button', { name: /Get Started/ })).toBeNull()
  })
})

describe('home API examples', () => {
  afterEach(() => {
    config.serverAddress = ''
    vi.useRealTimers()
  })

  it('uses the configured address for every protocol and reacts to configuration updates', () => {
    vi.useFakeTimers()
    config.serverAddress = ' https://gateway.example.com/proxy/ '
    const { container, rerender } = render(
      <>
        <HeroTerminalDemo />
        <JevShowcase />
      </>
    )

    expect(container.textContent).toContain(
      'curl https://gateway.example.com/proxy/v1/decisions'
    )
    for (const [tab, endpoint] of [
      ['Chat', '/v1/chat/completions'],
      ['Responses', '/v1/responses'],
      ['Claude', '/v1/messages'],
      ['Gemini', '/v1beta/models/{model}:generateContent'],
    ]) {
      act(() => {
        screen.getByRole('button', { name: tab }).click()
      })
      act(() => {
        vi.runOnlyPendingTimers()
      })
      expect(container.textContent).toContain(
        `curl -X POST "https://gateway.example.com/proxy${endpoint}"`
      )
    }

    config.serverAddress = 'https://updated.example.com'
    rerender(
      <>
        <HeroTerminalDemo />
        <JevShowcase />
      </>
    )
    expect(container.textContent).toContain(
      'curl https://updated.example.com/v1/decisions'
    )
    expect(container.textContent).toContain(
      'curl -X POST "https://updated.example.com/v1beta/models/{model}:generateContent"'
    )
    expect(container.textContent).not.toContain('gateway.example.com')
  })

  it('falls back to the current origin when no address is configured', () => {
    const { container } = render(
      <>
        <HeroTerminalDemo />
        <JevShowcase />
      </>
    )
    expect(container.textContent).toContain(
      `curl -X POST "${window.location.origin}/v1/chat/completions"`
    )
    expect(container.textContent).toContain(
      `curl ${window.location.origin}/v1/decisions`
    )
  })
})

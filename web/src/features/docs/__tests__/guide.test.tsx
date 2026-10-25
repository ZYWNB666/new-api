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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps, ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'

import { Docs } from '../index'

vi.mock('@tanstack/react-router', () => ({
  Link: (props: ComponentProps<'a'> & { to: string }) => {
    const { to, ...rest } = props
    return <a href={to} {...rest} />
  },
}))
vi.mock('@/components/layout/components/public-layout', () => ({
  PublicLayout: (props: { children: ReactNode }) => props.children,
}))
vi.mock('@/components/layout/components/footer', () => ({ Footer: () => null }))

describe('documentation guide', () => {
  it('links the on-this-page directory to every section without losing authenticated console destinations', () => {
    render(<Docs />)
    const directory = screen.getByRole('navigation', { name: 'On this page' })
    for (const link of within(directory).getAllByRole('link')) {
      const target = document.querySelector(
        link.getAttribute('href') ?? '#missing-section'
      )
      expect(target).toHaveClass('docs-section')
      expect(target).toHaveTextContent(link.textContent ?? '')
    }
    expect(screen.getByRole('button', { name: /Get API Key/ })).toHaveAttribute(
      'href',
      '/keys'
    )
    expect(screen.getByRole('button', { name: 'Usage Logs' })).toHaveAttribute(
      'href',
      '/usage-logs'
    )
  })

  it('copies the selected cURL example with placeholders and keeps code independently scrollable', async () => {
    const user = userEvent.setup()
    render(<Docs />)
    const code = within(screen.getByRole('tabpanel')).getByLabelText('cURL')
    expect(code).toHaveClass('overflow-x-auto')
    expect(code).toHaveAttribute('tabindex', '0')
    await user.click(
      within(screen.getByRole('tabpanel')).getByRole('button', {
        name: 'Copy example',
      })
    )
    expect(await navigator.clipboard.readText()).toBe(code.textContent)
    expect(code).toHaveTextContent('Bearer YOUR_API_KEY')
    expect(code).toHaveTextContent('YOUR_MODEL_NAME')
  })

  it('switches SDK tabs using the keyboard and copies the full server-side example', async () => {
    const user = userEvent.setup()
    render(<Docs />)
    const curl = screen.getByRole('tab', { name: 'cURL' })
    curl.focus()
    await user.keyboard('{ArrowRight}{Enter}')
    expect(
      screen.getByRole('tab', { name: 'OpenAI SDK · Python' })
    ).toHaveAttribute('aria-selected', 'true')
    const code = await screen.findByLabelText('Python')
    expect(code).toHaveTextContent('os.environ["NEW_API_KEY"]')
    await user.click(
      within(screen.getByRole('tabpanel')).getByRole('button', {
        name: 'Copy example',
      })
    )
    expect(await navigator.clipboard.readText()).toBe(code.textContent)
  })

  it('shows troubleshooting statuses and preserves the project documentation link', () => {
    render(<Docs />)
    expect(screen.getByText(/Save X-Nawei-Request-Id/)).toBeVisible()
    expect(screen.queryByText(/X-Oneapi-Request-Id/)).not.toBeInTheDocument()
    expect(screen.getByRole('table')).toHaveClass('min-w-[32rem]')
    for (const status of ['401', '403', '429', '5xx']) {
      expect(screen.getByText(status)).toBeVisible()
    }
    expect(
      screen.getByRole('link', { name: 'Project documentation' })
    ).toHaveAttribute('href', 'https://docs.newapi.ai/')
    expect(screen.getByLabelText('Python · stream')).toHaveTextContent(
      'stream=True'
    )
  })
})

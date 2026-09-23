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
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { Footer } from '../components/footer'

const systemConfig = vi.hoisted(() => ({
  systemName: 'Example AI',
  logo: '/example-logo.png',
  footerHtml: '',
  demoSiteEnabled: false,
}))

vi.mock('@tanstack/react-router', () => ({
  Link: (props: ComponentProps<'a'> & { to: string }) => {
    const { to, ...linkProps } = props
    return <a href={to} {...linkProps} />
  },
}))

vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => systemConfig,
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: null }),
}))

describe('public footer company copyright', () => {
  beforeEach(() => {
    systemConfig.systemName = 'Example AI'
    systemConfig.logo = '/example-logo.png'
    systemConfig.footerHtml = ''
    systemConfig.demoSiteEnabled = false
  })

  it('shows the configured brand and company at the bottom of the default footer', () => {
    render(<Footer />)

    const year = new Date().getFullYear()
    expect(
      screen.getByText(
        new RegExp(
          `© ${year} Example AI · 上海纳维智算科技有限公司\\. footer\\.defaultCopyright`
        )
      )
    ).toBeInTheDocument()
  })

  it('keeps the configured brand and company beside custom footer content', () => {
    systemConfig.footerHtml = '<p>Custom legal notice</p>'

    render(<Footer />)

    expect(screen.getByText('Custom legal notice')).toBeInTheDocument()
    expect(
      screen.getByText(/Example AI · 上海纳维智算科技有限公司/)
    ).toBeInTheDocument()
  })
})

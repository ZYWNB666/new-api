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
import { describe, expect, it } from 'vitest'

import { isHttpUrl, isLikelyHtml } from './content-format'

describe('content format detection', () => {
  it('recognizes standalone HTTP URLs', () => {
    expect(isHttpUrl('https://api.netwebs.top/user-agreement')).toBe(true)
    expect(isHttpUrl('# User agreement')).toBe(false)
  })

  it('recognizes actual HTML documents and fragments', () => {
    expect(isLikelyHtml('<!doctype html><html><body>Terms</body></html>')).toBe(
      true
    )
    expect(isLikelyHtml('<section class="terms">Terms</section>')).toBe(true)
  })

  it('does not mistake Markdown autolinks for HTML', () => {
    const agreement = `# 维流 Token 平台用户服务协议

网站：<https://api.netwebs.top/>

邮箱：<2261665821@qq.com>`

    expect(isLikelyHtml(agreement)).toBe(false)
  })
})

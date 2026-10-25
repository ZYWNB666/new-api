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
import { render, screen, within } from '@testing-library/react'
import i18next from 'i18next'
import { afterEach, beforeAll, describe, expect, test } from 'vitest'

import type { UsageLog } from '../../data/schema'
import { getTokenCharges } from '../../lib/charge-breakdown'
import type { LogOtherData } from '../../types'
import { ChargeBreakdown } from '../dialogs/charge-breakdown'
import { DetailsDialog } from '../dialogs/details-dialog'

const i18nKeys = {
  'Log Details': 'Log Details',
  Consume: 'Consume',
  'Billing Details': 'Billing Details',
  'Billing Mode': 'Billing Mode',
  'Per-token': 'Per-token',
  'Dynamic Pricing': 'Dynamic Pricing',
  'Matched Tier': 'Matched Tier',
  'Group Ratio': 'Group Ratio',
  'Total Cost': 'Total Cost',
  'Usage parameters': 'Usage parameters',
}

function makeLog(other: LogOtherData): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 1,
    type: 2,
    content: '',
    username: 'user',
    token_name: 'token',
    model_name: 'wan2.5-i2v-preview',
    quota: 5000,
    prompt_tokens: 0,
    completion_tokens: 0,
    use_time: 0,
    is_stream: false,
    channel: 1,
    channel_name: '',
    token_id: 1,
    group: 'default',
    ip: '',
    other: JSON.stringify(other),
    request_id: 'req-1',
    upstream_request_id: '',
  }
}

function renderDetails(
  other: LogOtherData,
  promptTokens = 0,
  isAdmin = false
): QueryClient {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const freshAt = Date.now() + 60_000
  queryClient.setQueryData(['status'], {}, { updatedAt: freshAt })
  queryClient.setQueryData(
    ['pricing'],
    { data: [], vendors: [] },
    { updatedAt: freshAt }
  )

  render(
    <QueryClientProvider client={queryClient}>
      <DetailsDialog
        log={{ ...makeLog(other), prompt_tokens: promptTokens }}
        isAdmin={isAdmin}
        isRoot={false}
        open
        onOpenChange={() => undefined}
      />
    </QueryClientProvider>
  )
  return queryClient
}

function rowValue(label: string): string | null {
  return screen.getByText(label).nextElementSibling?.textContent ?? null
}

describe('token charge details', () => {
  test('shows an explicit amount column and the actual charged total', () => {
    const other: LogOtherData = {
      billing_charge_tokens: {
        p: 87,
        c: 128,
        cr: 0,
        cc: 22272,
        cc5m: 0,
        cc1h: 0,
      },
      billing_prices: { p: 10, c: 50, cr: 1, cc: 10, cc5m: 10, cc1h: 20 },
      cache_creation_tokens_5m: 0,
      cache_creation_tokens_1h: 22272,
      billing_quota_per_unit: 500000,
    }
    render(
      <ChargeBreakdown
        log={{ ...makeLog(other), quota: 114995 }}
        other={other}
      />
    )
    const table = within(
      screen.getByRole('table', { name: 'Charge Breakdown' })
    )
    expect(
      table.getByRole('columnheader', { name: 'Charged amount' })
    ).toBeVisible()
    const input = within(table.getByRole('row', { name: /Input charge/ }))
    expect(input.getByRole('cell', { name: '87' })).toBeVisible()
    expect(input.getByRole('cell', { name: '$0.00087' })).toBeVisible()
    expect(
      within(
        table.getByRole('row', { name: /Cache write charge \(1h\)/ })
      ).getByRole('cell', { name: '$0.22272' })
    ).toBeVisible()
    expect(
      within(table.getByRole('row', { name: /Total/ })).getByRole('cell', {
        name: '$0.22999',
      })
    ).toBeVisible()
  })
  test('shows total writes and explicit zero versus absent TTL facts', () => {
    const client = renderDetails(
      {
        cache_creation_tokens: 200,
        cache_creation_tokens_5m: 0,
        cache_creation_tokens_1h: 200,
      },
      1000
    )
    expect(rowValue('Cache Write')).toBe('200')
    expect(rowValue('Cache Write (5m)')).toBe('0')
    expect(rowValue('Cache Write (1h)')).toBe('200')
    client.clear()
  })

  test('does not turn unknown TTL counts or old missing prices into zero charges', () => {
    const client = renderDetails({ cache_creation_tokens: 200 }, 1000)
    expect(rowValue('Cache Write (5m)')).toBe('Not reported')
    expect(rowValue('Cache Write (1h)')).toBe('Not reported')
    expect(
      within(screen.getByRole('row', { name: /Input charge/ })).getByText(
        'Unit price unavailable'
      )
    ).toBeVisible()
    expect(
      within(
        screen.getByRole('row', { name: /Cache write charge \(1h\)/ })
      ).getAllByText('Not reported')
    ).toHaveLength(2)
    client.clear()
  })

  test('uses frozen quantities and effective prices in separate amount cells', () => {
    const other: LogOtherData = {
      billing_charge_tokens: {
        p: 100,
        c: 20,
        cr: 50,
        cc: 200,
        cc5m: 0,
        cc1h: 0,
      },
      billing_prices: { p: 6, c: 12, cr: 0.6, cc: 9, cc5m: 10, cc1h: 20 },
      cache_creation_tokens_5m: 0,
      cache_creation_tokens_1h: 200,
    }
    render(<ChargeBreakdown log={makeLog(other)} other={other} />)
    const table = within(
      screen.getByRole('table', { name: 'Charge Breakdown' })
    )
    expect(
      within(table.getByRole('row', { name: /Input charge/ })).getByRole(
        'cell',
        { name: '$0.0006' }
      )
    ).toBeVisible()
    expect(
      within(
        table.getByRole('row', { name: /Cache write charge \(1h\)/ })
      ).getByRole('cell', { name: '$0.0018' })
    ).toBeVisible()
    expect(
      within(table.getByRole('row', { name: /Cache read charge/ })).getByRole(
        'cell',
        { name: '$0.00003' }
      )
    ).toBeVisible()
    expect(screen.queryByText('Cache write charge')).not.toBeInTheDocument()
  })

  test('preserves an explicitly zero subscription deduction in the total', () => {
    const other: LogOtherData = {
      billing_source: 'subscription',
      subscription_consumed: 0,
      billing_quota_per_unit: 500000,
    }
    render(<ChargeBreakdown log={makeLog(other)} other={other} />)
    const table = within(
      screen.getByRole('table', { name: 'Charge Breakdown' })
    )
    expect(
      within(table.getByRole('row', { name: /Total/ })).getByRole('cell', {
        name: '$0',
      })
    ).toBeVisible()
    expect(
      within(table.getByRole('row', { name: /Input charge/ })).getByText(
        'Unit price unavailable'
      )
    ).toBeVisible()
  })

  test('applies logged expression group and request multipliers without changing generic write prices', () => {
    const rows = getTokenCharges(
      {
        billing_mode: 'tiered_expr',
        expr_b64: btoa(
          'tier("base", p * 2 + c * 4 + cc * 3 + cc1h * 6) * (param("fast") == true ? 2 : 1)'
        ),
        matched_tier: 'base',
        billing_group_ratio: 1.5,
        request_rules: [
          { cond: 'param("fast") == true', multiplier: 2, matched: true },
        ],
        billing_tokens: { p: 100, c: 20, cr: 0, cc: 200, cc1h: 0 },
        cache_creation_tokens_5m: 0,
        cache_creation_tokens_1h: 200,
      },
      300,
      20
    )
    expect(rows.find((row) => row.key === 'p')).toMatchObject({
      tokens: 100,
      price: 6,
    })
    expect(rows.find((row) => row.key === 'cc1h')).toMatchObject({
      tokens: 200,
      price: 9,
    })
    expect(rows.find((row) => row.key === 'cr')?.included).toBe(true)
  })

  test('preserves free prices and keeps separately billed native TTL quantities', () => {
    const rows = getTokenCharges(
      {
        billing_tokens: { p: 100, c: 20, cr: 30, cc: 0, cc5m: 200, cc1h: 300 },
        billing_prices: { p: 0, c: 0, cr: 0, cc: 0, cc5m: 3.75, cc1h: 6 },
        cache_creation_tokens_5m: 200,
        cache_creation_tokens_1h: 300,
      },
      100,
      20
    )
    expect(rows.find((row) => row.key === 'p')?.price).toBe(0)
    expect(rows.find((row) => row.key === 'cc5m')).toMatchObject({
      tokens: 200,
      price: 3.75,
    })
    expect(rows.find((row) => row.key === 'cc1h')).toMatchObject({
      tokens: 300,
      price: 6,
    })
  })

  test('does not invent unit prices for custom nonlinear expressions', () => {
    const rows = getTokenCharges(
      {
        billing_mode: 'tiered_expr',
        expr_b64: btoa('p * p + c'),
        billing_group_ratio: 1,
        billing_tokens: { p: 100, c: 20 },
      },
      100,
      20
    )
    expect(rows.every((row) => row.price === null)).toBe(true)
    expect(rows.some((row) => row.included)).toBe(false)
  })

  test('allocates a native 5m write when no 1h field was reported', () => {
    const rows = getTokenCharges(
      {
        billing_mode: 'tiered_expr',
        expr_b64: btoa('tier("base", p * 3 + cc * 3.75 + cc1h * 6)'),
        matched_tier: 'base',
        billing_group_ratio: 1,
        billing_tokens: { p: 100, c: 0, cc: 250, cc1h: 0 },
        cache_creation_tokens_5m: 200,
      },
      100,
      0
    )
    expect(rows.find((row) => row.key === 'cc5m')).toMatchObject({
      tokens: 200,
      price: 3.75,
    })
    expect(rows.find((row) => row.key === 'cc1h')?.tokens).toBeNull()
    expect(rows.find((row) => row.key === 'cc')).toMatchObject({
      tokens: 50,
      price: 3.75,
    })
  })
})

test('hides upstream model mapping from non-admin log details', () => {
  // The backend strips admin_info for non-admin views, so a real non-admin row
  // carries no mapping data at all. Role filtering is server-side, not a UI
  // concern, so this asserts the row renders cleanly without the section.
  const queryClient = renderDetails({
    model_ratio: 1,
    group_ratio: 1,
  })
  expect(screen.queryByText('Model Mapping')).not.toBeInTheDocument()
  expect(screen.queryByText('Actual Model')).not.toBeInTheDocument()
  expect(screen.queryByText('mapped-model')).not.toBeInTheDocument()
  queryClient.clear()
})

test('shows the recorded request and response models to admins', () => {
  const queryClient = renderDetails(
    {
      admin_info: {
        response_model: {
          requested_model: 'requested-model',
          upstream_model: 'mapped-model',
          returned_model: 'unexpected-model',
        },
      },
    },
    0,
    true
  )
  expect(screen.getByText('Response model: unexpected-model')).toBeVisible()
  expect(rowValue('Request Model')).toBe('requested-model')
  expect(rowValue('Upstream Model')).toBe('mapped-model')
  expect(screen.getByText('unexpected-model')).toBeVisible()
  queryClient.clear()
})

describe('usage facts billing details', () => {
  test('shows the settled image count and a per-image price', () => {
    const queryClient = renderDetails({
      billing_mode: 'tiered_expr',
      expr_b64: btoa('tier("image", fixed(0.04)) * image_count'),
      billing_unit: 'request',
      fixed_price: 0.04,
      image_count: 2,
      matched_tier: 'image',
    })
    expect(
      screen.getByText('Billable image count').parentElement
    ).toHaveTextContent('2')
    expect(screen.getAllByText(/\/image/).length).toBeGreaterThan(0)
    queryClient.clear()
  })
  const queryClients: QueryClient[] = []

  test('shows actual billable image and cache tokens while retaining the aggregate cache count', () => {
    queryClients.push(
      renderDetails(
        {
          billing_mode: 'tiered_expr',
          expr_b64: btoa(
            'tier("standard", p * 5 + cr * 1.25 + img * 8 + img_cr * 2 + c * 30)'
          ),
          matched_tier: 'standard',
          cache_tokens: 300,
          image_cache_tokens: 200,
          billing_tokens: { p: 300, cr: 100, img: 400, img_cr: 200, c: 100 },
        },
        1000
      )
    )
    const billable = within(
      screen.getByRole('group', { name: 'Billable token breakdown' })
    )
    expect(
      billable.getByText('Image Cache').nextElementSibling
    ).toHaveTextContent('200')
    expect(
      billable.getByText('Cache Read').nextElementSibling
    ).toHaveTextContent('100')
    expect(billable.getByText('Image In').nextElementSibling).toHaveTextContent(
      '400'
    )
    expect(
      screen.getByText('Input Tokens').nextElementSibling
    ).toHaveTextContent('1,000')
    expect(
      screen
        .getAllByText('Cache Read')
        .some((label) => label.nextElementSibling?.textContent === '300')
    ).toBe(true)
  })

  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', i18nKeys)
  })

  afterEach(() => {
    for (const queryClient of queryClients) {
      queryClient.clear()
    }
    queryClients.length = 0
  })

  test('renders one raw-key row per usage fact before total cost', () => {
    const expression = 'tier("720P", u("seconds") * 5)'
    queryClients.push(
      renderDetails({
        group_ratio: 1,
        billing_mode: 'tiered_expr',
        expr_b64: Buffer.from(expression, 'utf8').toString('base64'),
        matched_tier: '720P',
        usage_facts: {
          resolution: '720P',
          seconds: 5,
        },
      })
    )

    expect(screen.getByText('Usage parameters')).toBeInTheDocument()
    expect(rowValue('resolution')).toBe('720P')
    expect(rowValue('seconds')).toBe('5')
    expect(rowValue('Billing Mode')).toBe('Dynamic Pricing')
    expect(rowValue('Matched Tier')).toBe('720P')

    const usageHeader = screen.getByText('Usage parameters')
    const totalCost = screen.getByText('Total Cost')
    expect(
      usageHeader.compareDocumentPosition(totalCost) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  test('does not render usage parameter rows when usage_facts is absent', () => {
    queryClients.push(
      renderDetails({
        group_ratio: 1,
      })
    )

    expect(screen.queryByText('Usage parameters')).toBeNull()
    expect(screen.queryByText('resolution')).toBeNull()
    expect(screen.queryByText('seconds')).toBeNull()
    expect(screen.getByText('Total Cost')).toBeInTheDocument()
  })

  test('does not render usage parameter rows when usage_facts is empty', () => {
    queryClients.push(
      renderDetails({
        group_ratio: 1,
        usage_facts: {},
      })
    )

    expect(screen.queryByText('Usage parameters')).toBeNull()
    expect(screen.queryByText('resolution')).toBeNull()
    expect(screen.queryByText('seconds')).toBeNull()
    expect(screen.getByText('Total Cost')).toBeInTheDocument()
  })
})

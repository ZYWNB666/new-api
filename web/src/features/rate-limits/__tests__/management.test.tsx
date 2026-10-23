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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { type Rule, ruleSchema } from '../api'
import { RateLimits } from '../index'
import { RuleDialog } from '../rule-dialog'

const rule: Rule = {
  id: 'supplier',
  name: 'Shared supplier',
  scope: 'provider',
  subject_id: 0,
  model: '',
  channel_ids: [2, 3],
  rpm: 60,
  tpm: 1000,
  concurrent: 5,
  enabled: true,
}
function renderWithQueries(element: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const result = render(
    <QueryClientProvider client={client}>{element}</QueryClientProvider>
  )
  return { ...result, client }
}

describe('gateway rate limit management', () => {
  it('rejects unlimited-only, missing target, fractional and missing model rules', () => {
    expect(ruleSchema.safeParse(rule).success).toBe(true)
    expect(
      ruleSchema.safeParse({ ...rule, rpm: 0, tpm: 0, concurrent: 0 }).success
    ).toBe(false)
    expect(ruleSchema.safeParse({ ...rule, channel_ids: [] }).success).toBe(
      false
    )
    expect(ruleSchema.safeParse({ ...rule, rpm: 1.5 }).success).toBe(false)
    expect(
      ruleSchema.safeParse({
        ...rule,
        scope: 'user_model',
        subject_id: 1,
        model: '',
      }).success
    ).toBe(false)
  })
  it.each([
    ['Users', 'user', 'Customer One'],
    ['API Keys', 'api_key', 'Production key'],
    ['User + model', 'user', 'Customer One'],
    ['Supplier pool', 'provider', 'Supplier A'],
  ])(
    'opens scope %s and saves its selected target',
    async (scope, kind, name) => {
      const user = userEvent.setup()
      vi.spyOn(api, 'get').mockResolvedValue({
        data: { success: true, data: [{ id: 2, name }] },
      })
      const save = vi.fn().mockResolvedValue(undefined)
      renderWithQueries(
        <RuleDialog
          rule={null}
          pending={false}
          onClose={vi.fn()}
          onSave={save}
        />
      )
      await user.click(screen.getByRole('button', { name: 'Scope' }))
      expect(screen.getAllByRole('option')).toHaveLength(4)
      await user.click(screen.getByRole('option', { name: scope }))
      await user.click(
        screen.getByRole('combobox', {
          name: scope === 'Supplier pool' ? 'Select a target' : 'Target',
        })
      )
      await user.click(
        await screen.findByRole('option', { name: `${name} (#2)` })
      )
      expect(api.get).toHaveBeenCalledWith(
        '/api/option/gateway_limits/targets',
        { params: { kind, search: '' } }
      )
      await user.type(screen.getByLabelText('Name'), 'Selected target')
      await user.clear(screen.getByLabelText('RPM'))
      await user.type(screen.getByLabelText('RPM'), '60')
      if (scope === 'User + model') {
        await user.type(
          screen.getByLabelText('Model', { exact: true }),
          'test-model'
        )
      }
      await user.click(screen.getByRole('button', { name: /^Save$/ }))
      await waitFor(() =>
        expect(save).toHaveBeenCalledWith(
          expect.objectContaining(
            scope === 'Supplier pool' ? { channel_ids: [2] } : { subject_id: 2 }
          )
        )
      )
    }
  )
  it('keeps targets immutable and preserves the editor when saving fails', async () => {
    const user = userEvent.setup()
    const save = vi.fn().mockRejectedValue(new Error('Conflict'))
    const close = vi.fn()
    renderWithQueries(
      <RuleDialog rule={rule} pending={false} onClose={close} onSave={save} />
    )
    expect(screen.getByRole('combobox', { name: 'Scope' })).toBeDisabled()
    const input = screen.getByLabelText('Name')
    await user.clear(input)
    await user.type(input, 'Updated supplier')
    await user.click(screen.getByRole('button', { name: /^Save$/ }))
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith({ ...rule, name: 'Updated supplier' })
    )
    expect(screen.getByRole('dialog')).toBeVisible()
    expect(close).not.toHaveBeenCalled()
  })
  it('shows current usage and deletes a rule only after confirmation with its revision', async () => {
    const user = userEvent.setup()
    vi.spyOn(api, 'get').mockImplementation(async (url) => ({
      data: {
        success: true,
        data: String(url).endsWith('/usage')
          ? [{ id: rule.id, rpm: 2, tpm: 80, concurrent: 1 }]
          : {
              config: { revision: 7, rules: [rule] },
              backend: 'redis',
              legacy_enabled: false,
            },
      },
    }))
    const put = vi.spyOn(api, 'put').mockResolvedValue({
      data: {
        success: true,
        data: {
          config: { revision: 8, rules: [] },
          backend: 'redis',
          legacy_enabled: false,
        },
      },
    })
    renderWithQueries(<RateLimits />)
    expect(await screen.findByText('Shared supplier')).toBeVisible()
    await waitFor(() => expect(screen.getByText('2 / 60')).toBeVisible())
    await user.click(screen.getByRole('button', { name: 'Open menu' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
    expect(put).not.toHaveBeenCalled()
    const dialog = screen.getByRole('alertdialog')
    await user.click(within(dialog).getByRole('button', { name: 'Continue' }))
    await waitFor(() =>
      expect(put).toHaveBeenCalledWith('/api/option/gateway_limits', {
        revision: 7,
        rules: [],
      })
    )
    expect(await screen.findByText('No rate limit rules')).toBeVisible()
  })
  it('shows the configuration failure and allows retry', async () => {
    vi.spyOn(api, 'get').mockRejectedValue(new Error('Unavailable'))
    renderWithQueries(<RateLimits />)
    expect(await screen.findByRole('button', { name: 'Retry' })).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Add rate limit' })
    ).not.toBeInTheDocument()
  })
})

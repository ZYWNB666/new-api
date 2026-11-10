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
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { LogExportDialog } from '../log-export-dialog'

function renderExport(isAdmin = false) {
  useAuthStore.setState({
    auth: {
      ...useAuthStore.getState().auth,
      user: { id: 7, username: 'export-user', role: isAdmin ? 10 : 1 },
    },
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const result = render(
    <QueryClientProvider client={client}>
      <LogExportDialog
        isAdmin={isAdmin}
        params={{
          p: 4,
          page_size: 100,
          start_timestamp: 100,
          end_timestamp: 200,
          model_name: 'kimi-k3.1',
          group: 'vip',
          request_id: 'req-filter',
        }}
      />
    </QueryClientProvider>
  )
  return { ...result, client }
}

afterEach(() => {
  useAuthStore.setState({
    auth: { ...useAuthStore.getState().auth, user: null },
  })
})

test('exports selected fields and applied filters without page limits', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: [] } })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { task_id: 'systask_new' } },
  })
  renderExport()
  await user.click(screen.getByRole('button', { name: 'Export logs' }))
  const dialog = screen.getByRole('dialog')
  expect(within(dialog).queryByLabelText(/Username/)).not.toBeInTheDocument()
  await user.click(within(dialog).getByRole('button', { name: 'Deselect all' }))
  const create = within(dialog).getByRole('button', { name: 'Create export' })
  expect(create).toBeDisabled()
  await user.click(
    within(dialog).getByRole('checkbox', { name: /^Client Request ID/ })
  )
  await user.click(create)
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith('/api/log/export', {
      scope: 'self',
      mode: 'details',
      fields: ['client_request_id'],
      filter: {
        start_timestamp: 100,
        end_timestamp: 200,
        model_name: 'kimi-k3.1',
        group: 'vip',
        request_id: 'req-filter',
      },
    })
  )
})

test('admin can select all fields and reopen persisted background jobs', async () => {
  const user = userEvent.setup()
  const future = Math.floor(Date.now() / 1000) + 3600
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: [
        {
          task_id: 'systask_done',
          status: 'succeeded',
          processed: 1200000,
          created_at: 100,
          expires_at: future,
          self: false,
          fields: ['model_name'],
        },
        {
          task_id: 'systask_expired',
          status: 'succeeded',
          processed: 2,
          created_at: 99,
          expires_at: 1,
          self: false,
          fields: ['model_name'],
        },
        {
          task_id: 'systask_failed',
          status: 'failed',
          processed: 0,
          created_at: 98,
          expires_at: future,
          self: false,
          fields: ['model_name'],
        },
        {
          task_id: 'systask_self',
          status: 'succeeded',
          processed: 999,
          created_at: 97,
          expires_at: future,
          self: true,
          fields: ['model_name'],
        },
      ],
    },
  })
  renderExport(true)
  await user.click(screen.getByRole('button', { name: 'Export logs' }))
  await user.click(screen.getByRole('button', { name: 'Select all' }))
  expect(screen.getByRole('checkbox', { name: /^Username/ })).toBeChecked()
  await screen.findByText(/1,200,000 rows exported/)
  expect(screen.getAllByRole('button', { name: 'Download' })).toHaveLength(1)
  expect(screen.getByText(/Expired/)).toBeInTheDocument()
  expect(screen.getByText(/Export failed/)).toBeInTheDocument()
  expect(screen.queryByText(/999 rows exported/)).not.toBeInTheDocument()
  await user.keyboard('{Escape}')
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
  await user.click(screen.getByRole('button', { name: 'Export logs' }))
  expect(await screen.findByText(/1,200,000 rows exported/)).toBeInTheDocument()
})

test('failed history can be retried and duplicate submissions are disabled while pending', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get')
    .mockRejectedValueOnce(new Error('unavailable'))
    .mockResolvedValue({ data: { success: true, data: [] } })
  let resolvePost: (value: unknown) => void = () => undefined
  vi.spyOn(api, 'post').mockImplementation(
    () =>
      new Promise((resolve) => {
        resolvePost = resolve
      })
  )
  renderExport()
  await user.click(screen.getByRole('button', { name: 'Export logs' }))
  await user.click(await screen.findByRole('button', { name: 'Retry' }))
  await screen.findByText('No exports yet')
  const create = screen.getByRole('button', { name: 'Create export' })
  await user.click(create)
  expect(create).toBeDisabled()
  expect(create).toHaveAttribute('aria-busy', 'true')
  resolvePost({ data: { success: true, data: { task_id: 'systask_new' } } })
  await waitFor(() => expect(create).toBeEnabled())
})

test('detail export selects token and amount breakdowns by default', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: [] } })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { task_id: 'systask_new' } },
  })
  renderExport()
  await user.click(screen.getByRole('button', { name: 'Export logs' }))
  expect(screen.getByText(/up to 31 days for details/)).toBeInTheDocument()
  expect(
    screen.getByRole('checkbox', { name: /^Cache read tokens/ })
  ).toBeChecked()
  expect(
    screen.getByRole('checkbox', { name: /^Total amount \(USD\)/ })
  ).toBeChecked()
  await user.click(screen.getByRole('button', { name: 'Create export' }))
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/log/export',
      expect.objectContaining({
        mode: 'details',
        fields: expect.arrayContaining([
          'cache_read_tokens',
          'cache_write_tokens',
          'amount_usd',
          'input_amount_usd',
          'output_amount_usd',
          'cache_read_amount_usd',
          'cache_write_amount_usd',
        ]),
      })
    )
  )
})

test('statistics export submits time grain and permitted dimensions independently of detail selection', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: [] } })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { task_id: 'systask_new' } },
  })
  renderExport()
  await user.click(screen.getByRole('button', { name: 'Export logs' }))
  await user.click(screen.getByRole('button', { name: 'Deselect all' }))
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Export type' }),
    'statistics'
  )
  expect(
    screen.queryByRole('checkbox', { name: /^Channel ID/ })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('checkbox', { name: /^User ID/ })
  ).not.toBeInTheDocument()
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Time granularity' }),
    'hour'
  )
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Time zone' }),
    'Asia/Shanghai'
  )
  await user.click(screen.getByRole('checkbox', { name: 'Group' }))
  await user.click(screen.getByRole('button', { name: 'Create export' }))
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/log/export',
      expect.objectContaining({
        mode: 'statistics',
        granularity: 'hour',
        timezone: 'Asia/Shanghai',
        group_by: ['model_name', 'group'],
      })
    )
  )
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Time granularity' }),
    'all'
  )
  expect(screen.getByRole('combobox', { name: 'Time zone' })).toBeDisabled()
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Export type' }),
    'details'
  )
  expect(screen.getByRole('button', { name: 'Create export' })).toBeDisabled()
})

test('administrator statistics can group by user and channel', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: [] } })
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { task_id: 'systask_new' } },
  })
  renderExport(true)
  await user.click(screen.getByRole('button', { name: 'Export logs' }))
  await user.selectOptions(
    screen.getByRole('combobox', { name: 'Export type' }),
    'statistics'
  )
  await user.click(screen.getByRole('checkbox', { name: 'User ID' }))
  await user.click(screen.getByRole('checkbox', { name: 'Channel ID' }))
  await user.click(screen.getByRole('button', { name: 'Create export' }))
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/log/export',
      expect.objectContaining({
        scope: 'all',
        mode: 'statistics',
        group_by: ['model_name', 'user_id', 'channel'],
      })
    )
  )
})

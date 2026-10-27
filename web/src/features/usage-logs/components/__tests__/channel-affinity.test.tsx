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
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { usageLogSchema } from '../../data/schema'
import { useCommonLogsColumns } from '../columns/common-logs-columns'
import { UsageLogsProvider, useUsageLogsContext } from '../usage-logs-provider'

function ChannelFixture(props: { source: string; hash?: string }) {
  const columns = useCommonLogsColumns(true, false)
  const context = useUsageLogsContext()
  const table = useReactTable({
    data: [
      usageLogSchema.parse({
        id: 1,
        user_id: 2,
        created_at: 1,
        type: 2,
        content: '',
        model_name: 'glm',
        username: 'user',
        channel: 5,
        channel_name: 'office',
        token_name: '',
        group: 'default',
        quota: 0,
        use_time: 1,
        prompt_tokens: 1,
        completion_tokens: 1,
        other: JSON.stringify({
          admin_info: {
            channel_affinity: {
              rule_name: 'automatic session',
              key_source: props.source,
              key_fp: props.hash,
              using_group: 'default',
            },
          },
        }),
      }),
    ],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })
  const cell = table
    .getRowModel()
    .rows[0].getAllCells()
    .find((candidate) => candidate.column.id === 'channel')
  if (!cell) throw new Error('Channel column missing')
  return (
    <>
      <button type='button' onClick={() => context.setSensitiveVisible(false)}>
        Hide sensitive data
      </button>
      {flexRender(cell.column.columnDef.cell, cell.getContext())}
    </>
  )
}

function renderChannel(source: string, hash?: string) {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <UsageLogsProvider>
        <ChannelFixture source={source} hash={hash} />
      </UsageLogsProvider>
    </QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  localStorage.clear()
})

it.each([
  {
    source: 'prompt_hash',
    hash: 'a1b2c3d4',
    suffix: ' (PromptHash: a1b2c3d4)',
  },
  { source: 'automatic', hash: 'a1b2c3d4', suffix: '' },
  { source: 'gjson', hash: 'a1b2c3d4', suffix: '' },
  { source: 'prompt_hash', hash: undefined, suffix: '' },
])(
  'shows the prompt hash suffix only for a content-derived fingerprint ($source, $hash)',
  async ({ source, hash, suffix }) => {
    renderChannel(source, hash)
    await userEvent.hover(screen.getByText('#5'))
    expect(
      await screen.findByText(`Rule: automatic session${suffix}`)
    ).toBeVisible()
  }
)

it('masks the prompt fingerprint when sensitive data is hidden', async () => {
  renderChannel('prompt_hash', 'a1b2c3d4')
  await userEvent.click(
    screen.getByRole('button', { name: 'Hide sensitive data' })
  )
  await userEvent.hover(screen.getByText('#5'))
  expect(
    await screen.findByText('Rule: automatic session (PromptHash: ••••)')
  ).toBeVisible()
  expect(screen.queryByText(/a1b2c3d4/)).not.toBeInTheDocument()
})

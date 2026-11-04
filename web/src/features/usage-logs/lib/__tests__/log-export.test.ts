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
import { afterEach, expect, test, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth-store'

import { downloadLogExport, type LogExportTask } from '../log-export'

const task: LogExportTask = {
  task_id: 'systask_file',
  status: 'succeeded',
  processed: 1000000,
  bytes: 256 * 1024 * 1024,
  created_at: 1,
  expires_at: 9999999999,
  self: true,
  fields: ['model_name'],
}

afterEach(() => {
  vi.unstubAllGlobals()
  Reflect.deleteProperty(window, 'showSaveFilePicker')
  useAuthStore.setState({
    auth: {
      ...useAuthStore.getState().auth,
      accessToken: null,
      accessExpiresAt: null,
    },
  })
})

test('streams a large export to the chosen file without buffering a Blob', async () => {
  useAuthStore.setState({
    auth: {
      ...useAuthStore.getState().auth,
      accessToken: 'test-access',
      accessExpiresAt: Math.floor(Date.now() / 1000) + 3600,
    },
  })
  const write = vi.fn()
  const writable = new WritableStream({ write })
  const picker = vi
    .fn()
    .mockResolvedValue({ createWritable: async () => writable })
  Object.defineProperty(window, 'showSaveFilePicker', {
    configurable: true,
    value: picker,
  })
  const bytes = new TextEncoder().encode('model_name\r\nkimi-k3.1\r\n')
  const body = new ReadableStream({
    start(controller) {
      controller.enqueue(bytes)
      controller.close()
    },
  })
  const blob = vi.fn()
  const fetcher = vi.fn().mockResolvedValue({ ok: true, body, blob })
  vi.stubGlobal('fetch', fetcher)
  await downloadLogExport(task)
  expect(picker).toHaveBeenCalledWith(
    expect.objectContaining({ suggestedName: 'usage-logs-systask_file.csv' })
  )
  expect(write).toHaveBeenCalledWith(bytes, expect.anything())
  expect(blob).not.toHaveBeenCalled()
  expect(fetcher).toHaveBeenCalledWith(
    '/api/log/export/systask_file/download',
    expect.objectContaining({
      headers: expect.objectContaining({ Authorization: 'Bearer test-access' }),
    })
  )
})

test('does not allocate a large Blob when direct-to-disk saving is unavailable', async () => {
  const fetcher = vi.fn()
  vi.stubGlobal('fetch', fetcher)
  await expect(downloadLogExport(task)).rejects.toThrow('For large exports')
  expect(fetcher).not.toHaveBeenCalled()
})

test('canceling the file picker makes no download request', async () => {
  Object.defineProperty(window, 'showSaveFilePicker', {
    configurable: true,
    value: vi
      .fn()
      .mockRejectedValue(new DOMException('canceled', 'AbortError')),
  })
  const fetcher = vi.fn()
  vi.stubGlobal('fetch', fetcher)
  await expect(downloadLogExport(task)).rejects.toHaveProperty(
    'name',
    'AbortError'
  )
  expect(fetcher).not.toHaveBeenCalled()
})

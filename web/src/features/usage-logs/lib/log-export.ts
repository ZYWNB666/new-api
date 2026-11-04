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
import { t } from 'i18next'

import { api, getFreshAuthHeaders } from '@/lib/api'
import {
  createServerError,
  requireServerSuccess,
} from '@/lib/server-error-message'

import type { GetLogsParams } from '../types'

export const LOG_EXPORT_FIELDS = [
  { key: 'created_at', label: 'Time' },
  { key: 'request_id', label: 'Request ID' },
  { key: 'upstream_request_id', label: 'Upstream Request ID' },
  { key: 'type', label: 'Type' },
  { key: 'model_name', label: 'Model Name' },
  { key: 'token_name', label: 'Token Name' },
  { key: 'prompt_tokens', label: 'Input Tokens' },
  { key: 'completion_tokens', label: 'Output Tokens' },
  { key: 'quota', label: 'Quota' },
  { key: 'use_time', label: 'Duration' },
  { key: 'is_stream', label: 'Stream' },
  { key: 'group', label: 'Group' },
  { key: 'ip', label: 'IP Address' },
  { key: 'content', label: 'Content' },
  { key: 'user_id', label: 'User ID', admin: true },
  { key: 'username', label: 'Username', admin: true },
  { key: 'channel', label: 'Channel ID', admin: true },
] satisfies Array<{ key: string; label: string; admin?: boolean }>

export type LogExportTask = {
  task_id: string
  status: 'pending' | 'running' | 'succeeded' | 'failed'
  processed: number
  bytes: number
  created_at: number
  expires_at: number
  self: boolean
  fields: string[]
}

export async function listLogExports(): Promise<LogExportTask[]> {
  const response = await api.get('/api/log/export')
  return requireServerSuccess(response.data).data
}

export async function createLogExport(
  params: GetLogsParams,
  fields: string[],
  isAdmin: boolean
): Promise<LogExportTask> {
  const { p: _page, page_size: _pageSize, ...filter } = params
  const response = await api.post('/api/log/export', {
    scope: isAdmin ? 'all' : 'self',
    filter,
    fields,
  })
  return requireServerSuccess(response.data).data
}

// File System Access is progressively enhanced: supporting browsers stream
// directly to disk. Other browsers use a bounded Blob download.
type ExportFileHandle = {
  createWritable(): Promise<WritableStream<Uint8Array>>
}
type ExportWindow = Window & {
  showSaveFilePicker?: (options: {
    suggestedName: string
    types: Array<{ description: string; accept: Record<string, string[]> }>
  }) => Promise<ExportFileHandle>
}

export async function downloadLogExport(task: LogExportTask): Promise<void> {
  const filename = `usage-logs-${task.task_id}.csv`
  const picker = (window as ExportWindow).showSaveFilePicker
  let handle: ExportFileHandle | undefined
  if (picker) {
    // Invoke the picker before awaiting authentication, preserving user activation.
    handle = await picker.call(window, {
      suggestedName: filename,
      types: [{ description: 'CSV', accept: { 'text/csv': ['.csv'] } }],
    })
  } else if (task.bytes > 128 * 1024 * 1024) {
    throw new Error(
      t(
        'For large exports, use a browser that supports saving directly to disk.'
      )
    )
  }
  const headers = await getFreshAuthHeaders()
  const response = await fetch(
    `/api/log/export/${encodeURIComponent(task.task_id)}/download`,
    {
      headers,
      credentials: 'same-origin',
      cache: 'no-store',
    }
  )
  if (!response.ok) {
    const body: unknown = await response.json().catch(() => null)
    throw createServerError(body, t('Failed to download export'))
  }
  if (handle && response.body) {
    const writable = await handle.createWritable()
    await response.body.pipeTo(writable)
    return
  }
  const blob = await response.blob()
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.append(link)
  link.click()
  link.remove()
  // The navigation starts asynchronously; do not revoke the URL before it starts.
  setTimeout(() => URL.revokeObjectURL(url), 60_000)
}

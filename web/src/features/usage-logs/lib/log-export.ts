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
  { key: 'client_request_id', label: 'Client Request ID' },
  { key: 'upstream_request_id', label: 'Upstream Request ID' },
  { key: 'type', label: 'Type' },
  { key: 'model_name', label: 'Model Name' },
  { key: 'token_name', label: 'Token Name' },
  { key: 'token_id', label: 'Token ID' },
  { key: 'prompt_tokens', label: 'Input Tokens' },
  { key: 'completion_tokens', label: 'Output Tokens' },
  { key: 'quota', label: 'Quota' },
  { key: 'input_tokens_total', label: 'Total input tokens' },
  { key: 'cache_read_tokens', label: 'Cache read tokens' },
  { key: 'cache_write_tokens', label: 'Cache write tokens' },
  { key: 'cache_write_tokens_5m', label: 'Cache write tokens (5m)' },
  { key: 'cache_write_tokens_1h', label: 'Cache write tokens (1h)' },
  { key: 'image_input_tokens', label: 'Image input tokens' },
  { key: 'image_output_tokens', label: 'Image output tokens' },
  { key: 'audio_input_tokens', label: 'Audio input tokens' },
  { key: 'audio_output_tokens', label: 'Audio output tokens' },
  { key: 'billing_input_tokens', label: 'Billable input tokens' },
  { key: 'billing_output_tokens', label: 'Billable output tokens' },
  { key: 'amount_usd', label: 'Total amount (USD)' },
  { key: 'input_amount_usd', label: 'Input amount (USD)' },
  { key: 'output_amount_usd', label: 'Output amount (USD)' },
  { key: 'cache_read_amount_usd', label: 'Cache read amount (USD)' },
  { key: 'cache_write_amount_usd', label: 'Cache write amount (USD)' },
  { key: 'image_input_amount_usd', label: 'Image input amount (USD)' },
  { key: 'image_output_amount_usd', label: 'Image output amount (USD)' },
  { key: 'audio_input_amount_usd', label: 'Audio input amount (USD)' },
  { key: 'audio_output_amount_usd', label: 'Audio output amount (USD)' },
  { key: 'other_amount_usd', label: 'Other charges and rounding (USD)' },
  { key: 'billing_source', label: 'Billing source' },
  { key: 'billing_unit', label: 'Billing unit' },
  { key: 'billing_quota_per_unit', label: 'Quota units per USD' },
  { key: 'billing_tokens', label: 'Billing token details (JSON)' },
  { key: 'billing_prices', label: 'Effective token prices (USD per 1M, JSON)' },
  { key: 'billing_expression', label: 'Historical billing expression' },
  { key: 'subscription_consumed', label: 'Subscription usage (quota)' },
  { key: 'use_time', label: 'Duration' },
  { key: 'is_stream', label: 'Stream' },
  { key: 'group', label: 'Group' },
  { key: 'ip', label: 'IP Address' },
  { key: 'content', label: 'Content' },
  { key: 'user_id', label: 'User ID', admin: true },
  { key: 'username', label: 'Username', admin: true },
  { key: 'channel', label: 'Channel ID', admin: true },
] satisfies Array<{ key: string; label: string; admin?: boolean }>

export const LOG_EXPORT_DIMENSIONS = [
  { key: 'model_name', label: 'Model Name' },
  { key: 'user_id', label: 'User ID', admin: true },
  { key: 'channel', label: 'Channel ID', admin: true },
  { key: 'group', label: 'Group' },
  { key: 'token_id', label: 'Token ID' },
  { key: 'token_name', label: 'Token Name' },
] satisfies Array<{ key: string; label: string; admin?: boolean }>

export type LogExportOptions = {
  mode: 'details' | 'statistics'
  granularity?: 'hour' | 'day' | 'month' | 'all'
  timezone?: string
  group_by?: string[]
}

export type LogExportTask = Partial<LogExportOptions> & {
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
  isAdmin: boolean,
  options: LogExportOptions = { mode: 'details' }
): Promise<LogExportTask> {
  const { p: _page, page_size: _pageSize, ...filter } = params
  const response = await api.post('/api/log/export', {
    scope: isAdmin ? 'all' : 'self',
    filter,
    fields,
    ...options,
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

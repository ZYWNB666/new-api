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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Download } from 'lucide-react'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  createLogExport,
  downloadLogExport,
  listLogExports,
  LOG_EXPORT_FIELDS,
  LOG_EXPORT_DIMENSIONS,
  type LogExportOptions,
} from '../lib/log-export'
import type { GetLogsParams } from '../types'

interface LogExportDialogProps {
  params: GetLogsParams
  isAdmin: boolean
}

export function LogExportDialog(props: LogExportDialogProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const id = useId()
  const [open, setOpen] = useState(false)
  const [selected, setSelected] = useState([
    'created_at',
    'request_id',
    'model_name',
    'prompt_tokens',
    'completion_tokens',
    'quota',
    'input_tokens_total',
    'cache_read_tokens',
    'cache_write_tokens',
    'cache_write_tokens_5m',
    'cache_write_tokens_1h',
    'image_input_tokens',
    'image_output_tokens',
    'audio_input_tokens',
    'audio_output_tokens',
    'billing_input_tokens',
    'billing_output_tokens',
    'amount_usd',
    'input_amount_usd',
    'output_amount_usd',
    'cache_read_amount_usd',
    'cache_write_amount_usd',
    'image_input_amount_usd',
    'image_output_amount_usd',
    'audio_input_amount_usd',
    'audio_output_amount_usd',
    'other_amount_usd',
    'billing_source',
    'billing_unit',
    'billing_prices',
    'subscription_consumed',
  ])
  const [mode, setMode] = useState<LogExportOptions['mode']>('details')
  const [granularity, setGranularity] =
    useState<NonNullable<LogExportOptions['granularity']>>('day')
  const [timezone, setTimezone] = useState('UTC')
  const localTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const [groupBy, setGroupBy] = useState<string[]>(['model_name'])
  const dimensions = LOG_EXPORT_DIMENSIONS.filter(
    (field) => !('admin' in field) || props.isAdmin
  )
  const [downloading, setDownloading] = useState<string | null>(null)
  const ownerID = useAuthStore((state) => state.auth.user?.id)
  const queryClient = useQueryClient()
  const fields = LOG_EXPORT_FIELDS.filter(
    (field) => !('admin' in field) || props.isAdmin
  )
  const queryKey = ['log-exports', ownerID]
  const history = useQuery({
    queryKey,
    queryFn: listLogExports,
    enabled: open && ownerID != null,
    refetchInterval: (query) =>
      query.state.data?.some(
        (task) => task.status === 'pending' || task.status === 'running'
      )
        ? 3000
        : false,
  })
  const create = useMutation({
    mutationFn: () =>
      createLogExport(
        props.params,
        fields
          .filter((field) => selected.includes(field.key))
          .map((field) => field.key),
        props.isAdmin,
        mode === 'statistics'
          ? {
              mode,
              granularity,
              timezone,
              group_by: dimensions
                .filter((field) => groupBy.includes(field.key))
                .map((field) => field.key),
            }
          : { mode }
      ),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey })
    },
  })
  const hasFields = fields.some((field) => selected.includes(field.key))
  const tasks =
    history.data?.filter((task) => task.self === !props.isAdmin) ?? []

  return (
    <Dialog
      open={open}
      onOpenChange={setOpen}
      trigger={
        <Button variant='outline'>
          <Download aria-hidden='true' />
          {t('Export logs')}
        </Button>
      }
      title={t('Export logs')}
      description={t(
        'Export detailed logs or grouped statistics in the background. Closing this dialog will not stop the export.'
      )}
      bodyClassName='space-y-5'
      footer={
        <Button
          disabled={(mode === 'details' && !hasFields) || create.isPending}
          aria-busy={create.isPending}
          onClick={() => create.mutate()}
        >
          {t('Create export')}
        </Button>
      }
    >
      <p className='text-muted-foreground text-sm'>
        {t(
          'Exports use applied filters: up to 31 days for details or 366 days for statistics. Files are available for 24 hours. Detail times are UTC, duration is in seconds, and amounts are USD from historical billing snapshots. Missing values remain blank.'
        )}
      </p>
      <div className='space-y-2'>
        <Label htmlFor={`${id}-mode`}>{t('Export type')}</Label>
        <NativeSelect
          id={`${id}-mode`}
          value={mode}
          onChange={(event) =>
            setMode(event.target.value as LogExportOptions['mode'])
          }
        >
          <NativeSelectOption value='details'>
            {t('Detailed data')}
          </NativeSelectOption>
          <NativeSelectOption value='statistics'>
            {t('Statistical data')}
          </NativeSelectOption>
        </NativeSelect>
      </div>
      {mode === 'statistics' && (
        <div className='space-y-4'>
          <div className='grid gap-4 sm:grid-cols-2'>
            <div className='space-y-2'>
              <Label htmlFor={`${id}-granularity`}>
                {t('Time granularity')}
              </Label>
              <NativeSelect
                id={`${id}-granularity`}
                value={granularity}
                onChange={(event) =>
                  setGranularity(
                    event.target.value as NonNullable<
                      LogExportOptions['granularity']
                    >
                  )
                }
              >
                <NativeSelectOption value='hour'>
                  {t('Hourly')}
                </NativeSelectOption>
                <NativeSelectOption value='day'>
                  {t('Daily')}
                </NativeSelectOption>
                <NativeSelectOption value='month'>
                  {t('Monthly')}
                </NativeSelectOption>
                <NativeSelectOption value='all'>
                  {t('Entire period')}
                </NativeSelectOption>
              </NativeSelect>
            </div>
            <div className='space-y-2'>
              <Label htmlFor={`${id}-timezone`}>{t('Time zone')}</Label>
              <NativeSelect
                id={`${id}-timezone`}
                value={timezone}
                disabled={granularity === 'all'}
                onChange={(event) => setTimezone(event.target.value)}
              >
                <NativeSelectOption value='UTC'>UTC</NativeSelectOption>
                {localTimezone !== 'UTC' && (
                  <NativeSelectOption value={localTimezone}>
                    {localTimezone}
                  </NativeSelectOption>
                )}
                {localTimezone !== 'Asia/Shanghai' && (
                  <NativeSelectOption value='Asia/Shanghai'>
                    Asia/Shanghai
                  </NativeSelectOption>
                )}
              </NativeSelect>
            </div>
          </div>
          <fieldset className='space-y-3'>
            <legend className='text-sm font-medium'>{t('Group by')}</legend>
            <div className='grid gap-3 sm:grid-cols-2'>
              {dimensions.map((field) => (
                <div key={field.key} className='flex items-center gap-2'>
                  <Checkbox
                    id={`${id}-group-${field.key}`}
                    checked={groupBy.includes(field.key)}
                    onCheckedChange={(checked) =>
                      setGroupBy((current) =>
                        checked
                          ? [...current, field.key]
                          : current.filter((key) => key !== field.key)
                      )
                    }
                  />
                  <Label htmlFor={`${id}-group-${field.key}`}>
                    {t(field.label)}
                  </Label>
                </div>
              ))}
            </div>
          </fieldset>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Statistics include log counts, token details, quota, duration, and amount totals. Log types and billing sources are kept separate. A total remains blank if any contributing value is unknown; amount_missing_logs counts logs without a known total amount.'
            )}
          </p>
        </div>
      )}
      {mode === 'details' && (
        <fieldset className='space-y-3'>
          <legend className='text-sm font-medium'>{t('Export fields')}</legend>
          <div className='flex gap-2'>
            <Button
              size='sm'
              variant='ghost'
              onClick={() => setSelected(fields.map((field) => field.key))}
            >
              {t('Select all')}
            </Button>
            <Button size='sm' variant='ghost' onClick={() => setSelected([])}>
              {t('Deselect all')}
            </Button>
          </div>
          <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
            {fields.map((field) => (
              <div key={field.key} className='flex items-center gap-2'>
                <Checkbox
                  id={`${id}-${field.key}`}
                  checked={selected.includes(field.key)}
                  onCheckedChange={(checked) =>
                    setSelected((current) =>
                      checked
                        ? [...current, field.key]
                        : current.filter((key) => key !== field.key)
                    )
                  }
                />
                <Label htmlFor={`${id}-${field.key}`}>
                  {t(field.label)}{' '}
                  <span className='text-muted-foreground text-xs'>
                    ({field.key})
                  </span>
                </Label>
              </div>
            ))}
          </div>
        </fieldset>
      )}
      <section className='space-y-3' aria-label={t('Recent exports')}>
        <h3 className='text-sm font-medium'>{t('Recent exports')}</h3>
        {history.isPending && <p role='status'>{t('Loading...')}</p>}
        {history.isError && (
          <Button variant='outline' onClick={() => void history.refetch()}>
            {t('Retry')}
          </Button>
        )}
        {history.isSuccess && tasks.length === 0 && (
          <p className='text-muted-foreground text-sm'>{t('No exports yet')}</p>
        )}
        {tasks.map((task) => {
          const expired = task.expires_at <= Date.now() / 1000
          let status = t('Pending')
          if (task.status === 'running') status = t('Running')
          if (task.status === 'succeeded') {
            status = expired ? t('Expired') : t('Completed')
          }
          if (task.status === 'failed') {
            status = t(
              'Export failed. Narrow the filters and retry, or contact an administrator.'
            )
          }
          return (
            <div
              key={task.task_id}
              className='flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3'
            >
              <div className='min-w-0 space-y-1 text-sm'>
                <p>
                  {new Date(task.created_at * 1000).toLocaleString(locale)} ·{' '}
                  {task.mode === 'statistics'
                    ? t('Statistical data')
                    : t('Detailed data')}
                </p>
                <p role='status'>
                  {status} ·{' '}
                  {t('{{rows}} rows exported', {
                    rows: formatNumber(task.processed, locale),
                  })}
                </p>
              </div>
              {task.status === 'succeeded' && !expired && (
                <Button
                  variant='outline'
                  disabled={downloading != null}
                  aria-busy={downloading === task.task_id}
                  onClick={async () => {
                    setDownloading(task.task_id)
                    try {
                      await downloadLogExport(task)
                    } catch (error) {
                      if (
                        !(
                          error instanceof DOMException &&
                          error.name === 'AbortError'
                        )
                      ) {
                        handleServerError(error, t('Failed to download export'))
                      }
                    } finally {
                      setDownloading(null)
                    }
                  }}
                >
                  {t('Download')}
                </Button>
              )}
            </div>
          )
        })}
      </section>
    </Dialog>
  )
}

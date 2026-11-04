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
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

import {
  createLogExport,
  downloadLogExport,
  listLogExports,
  LOG_EXPORT_FIELDS,
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
  ])
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
        props.isAdmin
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
        'Export matching logs in the background. Choose CSV fields below; closing this dialog will not stop the export.'
      )}
      bodyClassName='space-y-5'
      footer={
        <Button
          disabled={!hasFields || create.isPending}
          aria-busy={create.isPending}
          onClick={() => create.mutate()}
        >
          {t('Create export')}
        </Button>
      }
    >
      <p className='text-muted-foreground text-sm'>
        {t(
          'Exports use the applied page filters, cover up to 7 days, and remain downloadable for 24 hours. Times are UTC, duration is in seconds, and quota is in raw quota units.'
        )}
      </p>
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
                <p>{new Date(task.created_at * 1000).toLocaleString(locale)}</p>
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

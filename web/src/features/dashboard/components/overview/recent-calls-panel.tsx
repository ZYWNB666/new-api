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
import { Link } from '@tanstack/react-router'
import { ArrowRight, History } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

const LOG_TYPE_ERROR = 5

function formatTime(ts: number): string {
  const d = new Date(ts * 1000)
  return d.toLocaleString(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

interface RecentCallsPanelProps {
  calls: UsageLog[]
  loading?: boolean
}

export function RecentCallsPanel(props: RecentCallsPanelProps) {
  const { t } = useTranslation()
  const calls = Array.isArray(props.calls) ? props.calls : []

  let body: ReactNode
  if (props.loading) {
    body = (
      <div className='space-y-2'>
        {['a', 'b', 'c', 'd'].map((id) => (
          <Skeleton key={id} className='h-12 w-full' />
        ))}
      </div>
    )
  } else if (calls.length === 0) {
    body = (
      <div className='text-muted-foreground flex flex-1 flex-col items-center justify-center gap-2 py-10 text-center'>
        <History className='size-8 opacity-40' />
        <p className='text-sm font-medium'>{t('No recent calls')}</p>
        <p className='max-w-xs text-xs'>
          {t('Your latest API calls will appear here.')}
        </p>
      </div>
    )
  } else {
    body = (
      <ul className='divide-border/60 divide-y'>
        {calls.map((call) => {
          const failed = call.type === LOG_TYPE_ERROR
          return (
            <li
              key={call.id}
              className='flex items-center justify-between gap-3 py-2.5'
            >
              <div className='flex min-w-0 items-center gap-2.5'>
                <span
                  className={cn(
                    'size-2 shrink-0 rounded-full',
                    failed ? 'bg-destructive' : 'bg-emerald-500'
                  )}
                />
                <div className='min-w-0'>
                  <p className='truncate text-sm font-medium'>
                    {call.model_name || '—'}
                  </p>
                  <p className='text-muted-foreground text-[11px]'>
                    {formatTime(call.created_at)}
                  </p>
                </div>
              </div>
              <div className='shrink-0 text-right'>
                <p className='text-xs font-medium tabular-nums'>
                  {formatNumber(
                    (call.prompt_tokens || 0) + (call.completion_tokens || 0)
                  )}{' '}
                  {t('tokens')}
                </p>
                <p
                  className={cn(
                    'text-[11px]',
                    failed
                      ? 'text-destructive'
                      : 'text-emerald-600 dark:text-emerald-400'
                  )}
                >
                  {failed ? t('Failed') : t('Success')}
                </p>
              </div>
            </li>
          )
        })}
      </ul>
    )
  }

  return (
    <div className='bg-card border-border/70 flex h-full flex-col rounded-2xl border p-5 shadow-xs'>
      <div className='mb-3 flex items-center justify-between'>
        <div className='flex items-center gap-3'>
          <span className='bg-primary/10 text-primary flex size-9 items-center justify-center rounded-xl'>
            <History className='size-5' strokeWidth={1.7} />
          </span>
          <h3 className='text-base font-semibold'>{t('Recent calls')}</h3>
        </div>
        <Link
          to='/usage-logs'
          className='text-muted-foreground hover:text-primary inline-flex items-center gap-1 text-xs font-medium'
        >
          {t('View more')}
          <ArrowRight className='size-3.5' />
        </Link>
      </div>
      {body}
    </div>
  )
}

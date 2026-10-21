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
import { ChartPie, Trophy } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { getDashboardChartColors } from '@/features/dashboard/lib/charts'
import { formatNumber } from '@/lib/format'

import type { ModelRank } from './use-overview-data'

function EmptyHint(props: { text: string }) {
  return (
    <div className='text-muted-foreground flex flex-1 items-center justify-center py-10 text-center text-xs'>
      {props.text}
    </div>
  )
}

interface ModelInsightsProps {
  topModels: ModelRank[]
  quotaShare: ModelRank[]
}

export function ModelInsights(props: ModelInsightsProps) {
  const { t } = useTranslation()

  const maxRequests = Math.max(1, ...props.topModels.map((m) => m.requests))
  const totalQuota = props.quotaShare.reduce((sum, m) => sum + m.quota, 0)
  const colors = getDashboardChartColors(Math.max(1, props.quotaShare.length))

  return (
    <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
      {/* Top 5 models */}
      <div className='bg-card border-border/70 flex flex-col rounded-2xl border p-5 shadow-xs'>
        <div className='mb-4 flex items-center gap-3'>
          <span className='bg-primary/10 text-primary flex size-9 items-center justify-center rounded-xl'>
            <Trophy className='size-5' strokeWidth={1.7} />
          </span>
          <h3 className='text-base font-semibold'>{t('Top models')}</h3>
        </div>
        {props.topModels.length === 0 ? (
          <EmptyHint text={t('Model ranking appears after API calls.')} />
        ) : (
          <ul className='space-y-3'>
            {props.topModels.map((model, i) => (
              <li key={model.name} className='flex items-center gap-3'>
                <span className='text-muted-foreground w-4 shrink-0 text-xs font-semibold tabular-nums'>
                  {i + 1}
                </span>
                <div className='min-w-0 flex-1'>
                  <div className='mb-1 flex items-center justify-between gap-2'>
                    <span className='truncate text-sm font-medium'>
                      {model.name}
                    </span>
                    <span className='text-muted-foreground shrink-0 text-xs tabular-nums'>
                      {formatNumber(model.requests)} {t('requests')}
                    </span>
                  </div>
                  <div className='bg-muted h-1.5 overflow-hidden rounded-full'>
                    <div
                      className='bg-primary h-full rounded-full'
                      style={{
                        width: `${Math.max(4, (model.requests / maxRequests) * 100)}%`,
                      }}
                    />
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* Model spend share */}
      <div className='bg-card border-border/70 flex flex-col rounded-2xl border p-5 shadow-xs'>
        <div className='mb-4 flex items-center gap-3'>
          <span className='bg-primary/10 text-primary flex size-9 items-center justify-center rounded-xl'>
            <ChartPie className='size-5' strokeWidth={1.7} />
          </span>
          <h3 className='text-base font-semibold'>{t('Model spend share')}</h3>
        </div>
        {props.quotaShare.length === 0 || totalQuota <= 0 ? (
          <EmptyHint text={t('Spend share appears after billed usage.')} />
        ) : (
          <div className='flex flex-1 flex-col justify-center gap-4'>
            <div className='flex h-3 w-full overflow-hidden rounded-full'>
              {props.quotaShare.map((model, i) => (
                <div
                  key={model.name}
                  className='h-full'
                  style={{
                    width: `${(model.quota / totalQuota) * 100}%`,
                    backgroundColor: colors[i % colors.length],
                  }}
                  title={`${model.name}: ${((model.quota / totalQuota) * 100).toFixed(1)}%`}
                />
              ))}
            </div>
            <ul className='grid grid-cols-2 gap-x-4 gap-y-2'>
              {props.quotaShare.map((model, i) => (
                <li
                  key={model.name}
                  className='flex items-center justify-between gap-2 text-xs'
                >
                  <span className='flex min-w-0 items-center gap-1.5'>
                    <span
                      className='size-2.5 shrink-0 rounded-full'
                      style={{ backgroundColor: colors[i % colors.length] }}
                    />
                    <span className='truncate'>{model.name}</span>
                  </span>
                  <span className='text-muted-foreground shrink-0 tabular-nums'>
                    {((model.quota / totalQuota) * 100).toFixed(1)}%
                  </span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </div>
  )
}

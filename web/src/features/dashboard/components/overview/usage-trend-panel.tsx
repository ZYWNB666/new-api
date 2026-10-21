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
import { BarChart3 } from 'lucide-react'
import { useId, useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { TrendMetric, TrendPoint, TrendRange } from './use-overview-data'

interface UsageTrendPanelProps {
  data: TrendPoint[]
  metric: TrendMetric
  range: TrendRange
  loading?: boolean
  onMetricChange: (metric: TrendMetric) => void
  onRangeChange: (range: TrendRange) => void
}

function TrendChart({
  points,
  metric,
}: {
  points: { label: string; value: number }[]
  metric: TrendMetric
}) {
  const gradientId = useId().replaceAll(':', '')
  const width = 640
  const height = 200
  const padding = { top: 16, right: 12, bottom: 28, left: 12 }
  const innerW = width - padding.left - padding.right
  const innerH = height - padding.top - padding.bottom
  const max = Math.max(1, ...points.map((p) => p.value))
  const stepX = points.length > 1 ? innerW / (points.length - 1) : 0
  const coords = points.map((p, i) => ({
    x: padding.left + i * stepX,
    y: padding.top + innerH - (p.value / max) * innerH,
  }))
  const linePath = coords
    .map((c, i) => `${i === 0 ? 'M' : 'L'}${c.x},${c.y}`)
    .join(' ')
  const areaPath =
    coords.length > 0
      ? `${linePath} L${coords.at(-1)?.x ?? 0},${padding.top + innerH} L${coords[0].x},${padding.top + innerH} Z`
      : ''
  const labelEvery = Math.max(1, Math.ceil(points.length / 7))

  const formatValue = (v: number) =>
    metric === 'quota' ? formatNumber(v / 500000) : formatNumber(v)

  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      className='h-52 w-full'
      role='img'
      aria-label={metric}
      preserveAspectRatio='none'
    >
      <defs>
        <linearGradient id={gradientId} x1='0' y1='0' x2='0' y2='1'>
          <stop offset='0%' stopColor='var(--primary)' stopOpacity='0.28' />
          <stop offset='100%' stopColor='var(--primary)' stopOpacity='0' />
        </linearGradient>
      </defs>
      {[0, 0.5, 1].map((r) => (
        <line
          key={r}
          x1={padding.left}
          x2={width - padding.right}
          y1={padding.top + innerH * r}
          y2={padding.top + innerH * r}
          stroke='var(--border)'
          strokeOpacity='0.4'
          strokeDasharray='4 4'
        />
      ))}
      {areaPath && <path d={areaPath} fill={`url(#${gradientId})`} />}
      {linePath && (
        <path
          d={linePath}
          fill='none'
          stroke='var(--primary)'
          strokeWidth='2'
          strokeLinejoin='round'
          strokeLinecap='round'
        />
      )}
      {coords.map((c, i) => (
        <g key={c.x}>
          <circle
            cx={c.x}
            cy={c.y}
            r='3'
            fill='var(--background)'
            stroke='var(--primary)'
            strokeWidth='2'
          />
          {i % labelEvery === 0 && (
            <text
              x={c.x}
              y={height - 8}
              textAnchor='middle'
              className='fill-muted-foreground'
              fontSize='10'
            >
              {points[i].label}
            </text>
          )}
        </g>
      ))}
      {points.length > 0 && (
        <text
          x={padding.left}
          y={padding.top - 2}
          className='fill-muted-foreground'
          fontSize='10'
        >
          {formatValue(max)}
        </text>
      )}
    </svg>
  )
}

export function UsageTrendPanel(props: UsageTrendPanelProps) {
  const { t } = useTranslation()

  const metrics: { id: TrendMetric; label: string }[] = [
    { id: 'quota', label: t('Spend') },
    { id: 'requests', label: t('Requests') },
    { id: 'tokens', label: t('Tokens') },
  ]

  const points = useMemo(
    () =>
      props.data.map((d) => {
        let value = d.tokens
        if (props.metric === 'quota') value = d.quota
        else if (props.metric === 'requests') value = d.requests
        return { label: d.label, value }
      }),
    [props.data, props.metric]
  )

  const isEmpty = !props.loading && points.every((p) => p.value === 0)

  let chartBody: ReactNode
  if (props.loading) {
    chartBody = <Skeleton className='h-52 w-full' />
  } else if (isEmpty) {
    chartBody = (
      <div className='flex flex-col items-center gap-2 py-12 text-center'>
        <BarChart3 className='text-muted-foreground/40 size-8' />
        <p className='text-sm font-medium'>{t('No usage data yet')}</p>
        <p className='text-muted-foreground max-w-xs text-xs'>
          {t('Usage data will appear here after your first API call.')}
        </p>
      </div>
    )
  } else {
    chartBody = <TrendChart points={points} metric={props.metric} />
  }

  return (
    <div className='bg-card border-border/70 flex h-full flex-col rounded-2xl border p-5 shadow-xs'>
      <div className='mb-4 flex flex-wrap items-start justify-between gap-3'>
        <div className='flex items-center gap-3'>
          <span className='bg-primary/10 text-primary flex size-9 items-center justify-center rounded-xl'>
            <BarChart3 className='size-5' strokeWidth={1.7} />
          </span>
          <h3 className='text-base font-semibold'>{t('Usage trend')}</h3>
        </div>
        <div className='flex items-center gap-1'>
          <div className='bg-muted flex rounded-lg p-0.5'>
            {([7, 30] as TrendRange[]).map((r) => (
              <button
                key={r}
                type='button'
                onClick={() => props.onRangeChange(r)}
                className={cn(
                  'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                  props.range === r
                    ? 'bg-background text-foreground shadow-xs'
                    : 'text-muted-foreground hover:text-foreground'
                )}
              >
                {t('Last {{days}} days', { days: r })}
              </button>
            ))}
          </div>
        </div>
      </div>

      <div className='mb-3 flex gap-1'>
        {metrics.map((m) => (
          <button
            key={m.id}
            type='button'
            onClick={() => props.onMetricChange(m.id)}
            className={cn(
              'rounded-lg px-3 py-1.5 text-xs font-medium transition-colors',
              props.metric === m.id
                ? 'bg-primary/10 text-primary'
                : 'text-muted-foreground hover:bg-muted hover:text-foreground'
            )}
          >
            {m.label}
          </button>
        ))}
      </div>

      <div className='relative flex flex-1 items-center justify-center'>
        {chartBody}
      </div>
    </div>
  )
}

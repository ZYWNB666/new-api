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
import { Activity, Coins, Gauge, Wallet, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

interface MetricCardProps {
  title: string
  value: string
  description: string
  icon: LucideIcon
  tone: string
  loading?: boolean
}

function MetricCard(props: MetricCardProps) {
  const Icon = props.icon
  return (
    <div className='bg-card border-border/70 landing-motion-card flex items-center gap-4 rounded-2xl border p-5 shadow-xs'>
      <span
        className={cn(
          'flex size-11 shrink-0 items-center justify-center rounded-xl',
          props.tone
        )}
      >
        <Icon className='size-5' strokeWidth={1.7} />
      </span>
      <div className='min-w-0'>
        <p className='text-muted-foreground truncate text-xs font-medium'>
          {props.title}
        </p>
        {props.loading ? (
          <Skeleton className='mt-1.5 h-7 w-24' />
        ) : (
          <p className='mt-0.5 truncate text-2xl font-bold tracking-tight tabular-nums'>
            {props.value}
          </p>
        )}
        <p className='text-muted-foreground/80 mt-0.5 truncate text-[11px]'>
          {props.description}
        </p>
      </div>
    </div>
  )
}

interface OverviewMetricsProps {
  todayQuota: number
  todayRequests: number
  todayTokens: number
  successRate: number | null
  loading?: boolean
}

export function OverviewMetrics(props: OverviewMetricsProps) {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const balance = Number(user?.quota ?? 0)

  const successText =
    props.successRate == null
      ? '—'
      : `${formatNumber(props.successRate * 100, undefined)}%`

  return (
    <div className='grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4'>
      <MetricCard
        title={t('Account balance')}
        value={formatQuotaWithCurrency(balance)}
        description={t('Available account balance')}
        icon={Wallet}
        tone='bg-primary/10 text-primary'
      />
      <MetricCard
        title={t("Today's spend")}
        value={formatQuotaWithCurrency(props.todayQuota)}
        description={t('{{count}} tokens today', {
          count: formatNumber(props.todayTokens),
        })}
        icon={Coins}
        tone='bg-amber-500/10 text-amber-600 dark:text-amber-400'
        loading={props.loading}
      />
      <MetricCard
        title={t("Today's requests")}
        value={formatNumber(props.todayRequests)}
        description={t('{{count}} tokens today', {
          count: formatNumber(props.todayTokens),
        })}
        icon={Activity}
        tone='bg-violet-500/10 text-violet-600 dark:text-violet-400'
        loading={props.loading}
      />
      <MetricCard
        title={t('Success rate')}
        value={successText}
        description={t('Platform-wide, last 24 hours')}
        icon={Gauge}
        tone='bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
      />
    </div>
  )
}

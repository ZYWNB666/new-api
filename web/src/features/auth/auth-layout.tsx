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
import { Activity, Boxes, Gauge, TrendingUp, WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { LanguageSwitcher } from '@/components/language-switcher'
import { ThemeSwitch } from '@/components/theme-switch'
import { Skeleton } from '@/components/ui/skeleton'
import { useSystemConfig } from '@/hooks/use-system-config'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

type AuthLayoutProps = {
  children: React.ReactNode
  compact?: boolean
}

const SHOWCASE_MODELS = [
  { name: 'DeepSeek', icon: 'DeepSeek.Color', share: '42.1%' },
  { name: 'Qwen', icon: 'Qwen.Color', share: '31.8%' },
  { name: 'Zhipu GLM', icon: 'Zhipu.Color', share: '16.7%' },
  { name: 'Others', icon: '', share: '9.4%' },
] as const

const CONNECTED_MODELS = [
  { name: 'DeepSeek', icon: 'DeepSeek.Color' },
  { name: 'GLM', icon: 'Zhipu.Color' },
  { name: 'Qwen', icon: 'Qwen.Color' },
] as const

function AuthStatsShowcase() {
  const { t } = useTranslation()

  return (
    <div className='space-y-4'>
      {/* Compact gateway-to-models connection */}
      <div className='border-border/60 bg-card/70 flex items-center justify-center gap-2 rounded-2xl border px-4 py-3'>
        <span className='bg-muted text-muted-foreground rounded-lg px-2.5 py-1 text-[11px] font-semibold'>
          {t('Your app')}
        </span>
        <span className='text-primary/50'>→</span>
        {CONNECTED_MODELS.map((model) => (
          <span
            key={model.name}
            className='border-border/60 bg-background flex items-center gap-1.5 rounded-lg border px-2 py-1 text-[11px] font-semibold'
          >
            {getLobeIcon(model.icon, 16)}
            {model.name}
          </span>
        ))}
      </div>

      {/* Usage stats cards */}
      <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
        <div className='border-border/60 bg-card/70 rounded-2xl border p-4'>
          <div className='text-muted-foreground mb-1 flex items-center gap-1.5 text-[11px] font-medium'>
            <Activity className='size-3.5' />
            {t("Today's requests")}
          </div>
          <div className='flex items-baseline gap-2'>
            <span className='text-2xl font-bold tracking-tight tabular-nums'>
              2,847,945
            </span>
            <span className='text-emerald-600 dark:text-emerald-400 flex items-center gap-0.5 text-xs font-semibold'>
              <TrendingUp className='size-3' />
              +12.6%
            </span>
          </div>
        </div>

        <div className='border-border/60 bg-card/70 rounded-2xl border p-4'>
          <div className='text-muted-foreground mb-2 text-[11px] font-medium'>
            {t('Model call distribution')}
          </div>
          <div className='mb-2 flex h-1.5 w-full overflow-hidden rounded-full'>
            <span className='bg-primary' style={{ width: '42.1%' }} />
            <span className='bg-primary/70' style={{ width: '31.8%' }} />
            <span className='bg-primary/45' style={{ width: '16.7%' }} />
            <span className='bg-muted-foreground/30' style={{ width: '9.4%' }} />
          </div>
          <ul className='space-y-1'>
            {SHOWCASE_MODELS.map((model) => (
              <li
                key={model.name}
                className='flex items-center justify-between text-[11px]'
              >
                <span className='flex items-center gap-1.5'>
                  {model.icon ? getLobeIcon(model.icon, 13) : null}
                  {model.name === 'Others' ? t('Other') : model.name}
                </span>
                <span className='text-muted-foreground tabular-nums'>
                  {model.share}
                </span>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </div>
  )
}

export function AuthLayout(props: AuthLayoutProps) {
  const { t } = useTranslation()
  const { systemName, logo, loading } = useSystemConfig()

  return (
    <div className='auth-page public-surface flex flex-col px-4 sm:px-8'>
      <header className='mx-auto flex w-full max-w-6xl items-center justify-between gap-3 py-5 sm:py-7'>
        <Link
          to='/'
          className='flex min-w-0 items-center gap-2 transition-opacity hover:opacity-80'
        >
          <div className='relative h-8 w-8 shrink-0'>
            {loading ? (
              <Skeleton className='absolute inset-0 rounded-full' />
            ) : (
              <img
                src={logo}
                alt={t('Logo')}
                className='h-8 w-8 rounded-full object-cover'
              />
            )}
          </div>
          {loading ? (
            <Skeleton className='h-6 w-24' />
          ) : (
            <span className='truncate text-lg font-semibold'>{systemName}</span>
          )}
        </Link>
        <div className='flex shrink-0 items-center gap-1'>
          <LanguageSwitcher />
          <ThemeSwitch />
        </div>
      </header>
      <main
        className={cn(
          'mx-auto grid w-full max-w-6xl flex-1 items-center gap-10 py-6 sm:py-10',
          !props.compact && 'lg:grid-cols-[1fr_1fr] lg:gap-16'
        )}
      >
        {!props.compact && (
          <aside
            className='hidden min-w-0 space-y-7 lg:block'
            aria-label={t('AI Application Infrastructure Foundation')}
          >
            <div className='space-y-4'>
              <p className='text-primary text-sm font-semibold'>
                {t('AI Application Infrastructure Foundation')}
              </p>
              <h2 className='text-3xl leading-tight font-bold tracking-tight'>
                {t('One gateway, many possibilities')}
              </h2>
              <p className='text-muted-foreground leading-7'>
                {t('Compatible API routes for common AI application workflows')}
              </p>
            </div>
            <ul className='space-y-3'>
              {[
                {
                  icon: <Boxes className='size-4' strokeWidth={1.7} />,
                  title: t('Unified API access'),
                  desc: t('One integration to call mainstream models and channels'),
                },
                {
                  icon: <Gauge className='size-4' strokeWidth={1.7} />,
                  title: t('Smart routing & load balancing'),
                  desc: t('Route traffic across channels with priority and retries'),
                },
                {
                  icon: <WalletCards className='size-4' strokeWidth={1.7} />,
                  title: t('Flexible billing & monitoring'),
                  desc: t('Track usage, cost and quota in real time'),
                },
              ].map((feature) => (
                <li key={feature.title} className='flex items-start gap-3'>
                  <span className='bg-primary/10 text-primary flex size-8 shrink-0 items-center justify-center rounded-lg'>
                    {feature.icon}
                  </span>
                  <div className='min-w-0'>
                    <p className='text-sm font-medium'>{feature.title}</p>
                    <p className='text-muted-foreground text-xs leading-relaxed'>
                      {feature.desc}
                    </p>
                  </div>
                </li>
              ))}
            </ul>
            <AuthStatsShowcase />
          </aside>
        )}
        <div className='bg-card border-border/80 shadow-primary/5 mx-auto w-full max-w-[480px] min-w-0 rounded-2xl border p-5 shadow-xl sm:p-8'>
          {props.children}
        </div>
      </main>
    </div>
  )
}

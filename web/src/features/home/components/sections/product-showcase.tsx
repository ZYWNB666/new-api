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
import { Activity, Check, Gauge, Repeat2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import { HeroTerminalDemo } from '../hero-terminal-demo'

interface RouteNode {
  name: string
  icon: string
  latency: string
  rate: string
  status: 'healthy' | 'failed'
}

export function ProductShowcase() {
  const { t } = useTranslation()

  const routes: RouteNode[] = [
    {
      name: 'DeepSeek V3',
      icon: 'DeepSeek.Color',
      latency: '128 ms',
      rate: '99.9%',
      status: 'healthy',
    },
    {
      name: 'Qwen3',
      icon: 'Qwen.Color',
      latency: '156 ms',
      rate: '99.7%',
      status: 'healthy',
    },
    {
      name: 'Doubao Seed',
      icon: 'Doubao.Color',
      latency: '182 ms',
      rate: '99.5%',
      status: 'healthy',
    },
    {
      name: 'Kimi K2',
      icon: 'Moonshot.Color',
      latency: t('Auto failover'),
      rate: t('Failed'),
      status: 'failed',
    },
  ]

  const metrics = [
    { label: t('Route success rate'), value: '99.9%' },
    { label: t('Average latency'), value: '132 ms' },
    { label: t('Requests per minute'), value: '12.8K' },
    { label: t('Failovers today'), value: '23' },
  ]

  return (
    <section className='border-border/40 relative z-10 border-t px-4 py-16 sm:px-6 md:py-24'>
      <div className='mx-auto max-w-6xl'>
        <AnimateInView className='mb-12 text-center md:mb-16'>
          <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
            {t('Product Experience')}
          </p>
          <h2 className='text-2xl leading-tight font-bold tracking-tight md:text-3xl'>
            {t('Smart routing and live API, in one view')}
          </h2>
          <p className='text-muted-foreground mx-auto mt-3 max-w-xl text-sm leading-relaxed'>
            {t(
              'Verify routing in the Playground or your own client, and watch usage and cost update in real time.'
            )}
          </p>
        </AnimateInView>

        <div className='grid items-stretch gap-6 lg:grid-cols-2'>
          {/* Routing reliability card */}
          <AnimateInView
            animation='scale-in'
            className='bg-card border-border/80 flex flex-col rounded-2xl border p-6 shadow-xs md:p-8'
          >
            <div className='mb-6 flex items-center gap-3'>
              <span className='bg-primary/10 text-primary flex size-9 items-center justify-center rounded-lg'>
                <Repeat2 className='size-5' strokeWidth={1.6} />
              </span>
              <div>
                <h3 className='text-base font-semibold'>
                  {t('Smart routing & failover')}
                </h3>
                <p className='text-muted-foreground text-xs'>
                  {t('Traffic is routed to healthy model providers')}
                </p>
              </div>
            </div>

            <div className='space-y-2.5'>
              {routes.map((route) => (
                <div
                  key={route.name}
                  className={cn(
                    'landing-motion-card flex items-center gap-3 rounded-xl border px-3.5 py-3',
                    route.status === 'failed'
                      ? 'landing-motion-card--danger border-destructive/30 bg-destructive/5'
                      : 'border-border/50 bg-muted/20'
                  )}
                >
                  <span className='landing-card-icon flex size-8 shrink-0 items-center justify-center'>
                    {getLobeIcon(route.icon, 24)}
                  </span>
                  <div className='min-w-0 flex-1'>
                    <p className='truncate text-sm font-medium'>{route.name}</p>
                    <p className='text-muted-foreground text-xs'>
                      {t('Latency')} {route.latency}
                    </p>
                  </div>
                  <span
                    className={cn(
                      'flex items-center gap-1 text-xs font-semibold tabular-nums',
                      route.status === 'failed'
                        ? 'text-destructive'
                        : 'text-emerald-600 dark:text-emerald-400'
                    )}
                  >
                    {route.status === 'healthy' ? (
                      <Check className='size-3.5' />
                    ) : (
                      <Activity className='size-3.5' />
                    )}
                    {route.rate}
                  </span>
                </div>
              ))}
            </div>

            <div className='mt-6 grid grid-cols-2 gap-3 border-t pt-5 sm:grid-cols-4'>
              {metrics.map((m) => (
                <div key={m.label} className='text-center'>
                  <p className='text-lg font-bold tracking-tight'>{m.value}</p>
                  <p className='text-muted-foreground mt-0.5 text-[11px] leading-tight'>
                    {m.label}
                  </p>
                </div>
              ))}
            </div>
          </AnimateInView>

          {/* Live API demo */}
          <AnimateInView
            animation='scale-in'
            delay={120}
            className='bg-card border-border/80 flex flex-col rounded-2xl border p-6 shadow-xs md:p-8'
          >
            <div className='mb-6 flex items-center gap-3'>
              <span className='bg-primary/10 text-primary flex size-9 items-center justify-center rounded-lg'>
                <Gauge className='size-5' strokeWidth={1.6} />
              </span>
              <div>
                <h3 className='text-base font-semibold'>
                  {t('Use a familiar API')}
                </h3>
                <p className='text-muted-foreground text-xs'>
                  {t('OpenAI-compatible across Chat, Responses, Claude, Gemini')}
                </p>
              </div>
            </div>
            <div className='flex flex-1 items-center'>
              <HeroTerminalDemo />
            </div>
          </AnimateInView>
        </div>
      </div>
    </section>
  )
}

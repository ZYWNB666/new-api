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
import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useSystemConfig } from '@/hooks/use-system-config'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

const MODEL_NAMES = [
  'Qwen',
  'DeepSeek',
  'GLM',
  'Doubao',
  'Kimi',
  'Yi',
  'Baichuan',
  'Spark',
] as const

export function Hero(props: HeroProps) {
  const { t } = useTranslation()
  const { systemName } = useSystemConfig()

  return (
    <section className='relative z-10 overflow-hidden px-4 pt-28 pb-16 sm:px-6 md:pt-36 md:pb-24'>
      <div
        aria-hidden
        className='pointer-events-none absolute inset-x-0 top-0 -z-10 h-[34rem] opacity-70 dark:opacity-25'
        style={{
          background:
            'radial-gradient(ellipse 58% 52% at 50% 0%, color-mix(in oklch, var(--success) 32%, transparent) 0%, transparent 72%)',
        }}
      />
      <div
        aria-hidden
        className='landing-wave pointer-events-none absolute top-[-6rem] -z-10 h-[16rem] rounded-[100%] opacity-90 dark:opacity-45'
      />
      <div
        aria-hidden
        className='landing-wave landing-wave-alt pointer-events-none absolute top-[6rem] -z-10 h-[14rem] rounded-[100%] opacity-80 dark:opacity-35'
      />

      <div className='mx-auto flex max-w-6xl flex-col items-center text-center'>
        <div className='flex flex-col items-center'>
          <div
            className='landing-animate-fade-up border-success/20 bg-success/5 text-success mb-5 inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-[11px] font-semibold opacity-0 shadow-xs'
            style={{ animationDelay: '0ms' }}
          >
            <span className='relative flex size-1.5'>
              <span className='bg-success absolute inline-flex h-full w-full animate-ping rounded-full opacity-60' />
              <span className='bg-success relative inline-flex size-1.5 rounded-full' />
            </span>
            <span>{t('AI Application Infrastructure Foundation')}</span>
          </div>

          <h1
            className='landing-animate-fade-up text-[clamp(2.5rem,6vw,4rem)] leading-[1.08] font-bold tracking-[-0.04em]'
            style={{ animationDelay: '60ms' }}
          >
            <span className='block'>{systemName}</span>
            <span className='text-success mt-1 block'>
              {t('Vast Range of AI Models')}
            </span>
          </h1>
          <p
            className='landing-animate-fade-up text-muted-foreground mt-5 max-w-2xl text-base leading-7 opacity-0 md:text-lg'
            style={{ animationDelay: '120ms' }}
          >
            {t(
              'Access a vast selection of models via a standard, unified API protocol. Power AI applications, manage digital assets, and connect the Future.'
            )}
          </p>
          <p
            className='landing-animate-fade-up text-muted-foreground/60 mt-2 text-sm opacity-0'
            style={{ animationDelay: '140ms' }}
          >
            {t('Operated by 上海纳维智算科技有限公司')}
          </p>

          <div
            className='landing-animate-fade-up mt-7 flex flex-wrap items-center justify-center gap-3 opacity-0'
            style={{ animationDelay: '180ms' }}
          >
            {props.isAuthenticated ? (
              <Button
                className='group h-11 rounded-lg px-5 text-sm font-medium'
                render={<Link to='/dashboard' />}
              >
                {t('Go to Dashboard')}
                <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
              </Button>
            ) : (
              <>
                <Button
                  className='group h-11 rounded-lg px-5 text-sm font-medium'
                  render={<Link to='/sign-up' />}
                >
                  {t('Get Started')}
                  <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
                </Button>
                <Button
                  variant='outline'
                  className='border-border/50 hover:border-border hover:bg-muted/50 h-11 rounded-lg px-5 text-sm font-medium'
                  render={<Link to='/sign-in' />}
                >
                  {t('Sign In')}
                </Button>
                <Button
                  variant='outline'
                  className='border-border/50 hover:border-border hover:bg-muted/50 h-11 rounded-lg px-5 text-sm font-medium'
                  render={<Link to='/pricing' />}
                >
                  {t('View Pricing')}
                </Button>
              </>
            )}
          </div>
        </div>

        <div
          className='landing-model-strip landing-animate-fade-up border-border/70 bg-background/90 mt-16 flex w-full flex-wrap items-center justify-center gap-2 rounded-[1.75rem] border px-4 py-4 opacity-0 shadow-[0_16px_40px_-32px_rgba(15,23,42,0.45)] sm:justify-between sm:px-6'
          style={{ animationDelay: '240ms' }}
        >
          {MODEL_NAMES.map((model) => (
            <span
              key={model}
              className='bg-muted text-foreground rounded-full px-4 py-2 text-sm font-semibold'
            >
              {model}
            </span>
          ))}
        </div>

        <div className='mt-8 grid w-full max-w-2xl grid-cols-1 gap-3 sm:grid-cols-3'>
          {[
            ['50+', t('upstream services integrated')],
            ['100+', t('model billing support')],
            ['50+', t('compatible API routes')],
          ].map(([value, label]) => (
            <div
              key={label}
              className='landing-motion-card landing-metric-card border-border/80 bg-background rounded-2xl border px-5 py-5 shadow-xs'
            >
              <div className='text-success text-3xl font-bold tracking-tight'>
                {value}
              </div>
              <div className='text-muted-foreground mt-1 text-sm'>{label}</div>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

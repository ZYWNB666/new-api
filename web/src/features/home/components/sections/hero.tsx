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
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import { GatewayVisual } from '../gateway-visual'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

const MODELS = [
  { name: 'Qwen', icon: 'Qwen.Color' },
  { name: 'DeepSeek', icon: 'DeepSeek.Color' },
  { name: 'Zhipu GLM', icon: 'Zhipu.Color' },
  { name: 'Doubao', icon: 'Doubao.Color' },
  { name: 'Kimi', icon: 'Moonshot.Color' },
  { name: 'MiniMax', icon: 'MiniMax.Color' },
  { name: 'Baichuan', icon: 'Baichuan.Color' },
  { name: 'ERNIE', icon: 'Wenxin.Color' },
] as const

export function Hero(props: HeroProps) {
  const { t } = useTranslation()
  const { systemName } = useSystemConfig()

  return (
    <section
      className={cn(
        'public-surface relative z-10 overflow-hidden px-4 pt-28 pb-12 sm:px-6 md:pt-36 md:pb-20',
        props.className
      )}
    >
      <div
        aria-hidden
        className='pointer-events-none absolute inset-x-0 top-0 -z-10 h-[34rem] opacity-70 dark:opacity-25'
        style={{
          background:
            'radial-gradient(ellipse 58% 52% at 50% 0%, color-mix(in oklch, var(--primary) 32%, transparent) 0%, transparent 72%)',
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

      <div className='mx-auto grid max-w-6xl items-center gap-10 lg:grid-cols-[1.1fr_1fr] lg:gap-12'>
        <div className='flex min-w-0 flex-col items-start text-start'>
          <div
            className='landing-animate-fade-up border-primary/20 bg-primary/5 text-primary mb-5 inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-[11px] font-semibold opacity-0 shadow-xs'
            style={{ animationDelay: '0ms' }}
          >
            <span className='relative flex size-1.5'>
              <span className='bg-primary absolute inline-flex h-full w-full rounded-full opacity-60 motion-safe:animate-ping' />
              <span className='bg-primary relative inline-flex size-1.5 rounded-full' />
            </span>
            <span>{t('AI Application Infrastructure Foundation')}</span>
          </div>

          <h1
            className='landing-animate-fade-up text-[clamp(2.5rem,6vw,4rem)] leading-[1.08] font-bold tracking-[-0.04em]'
            style={{ animationDelay: '60ms' }}
          >
            <span className='block'>{systemName}</span>
            <span className='text-primary mt-1 block'>
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

          <div
            className='landing-animate-fade-up mt-7 flex flex-wrap items-center gap-3 opacity-0'
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
            <Button
              variant='ghost'
              className='h-11 px-3'
              render={<Link to='/docs' />}
            >
              {t('Documentation')}
            </Button>
          </div>
          <div className='text-muted-foreground mt-7 flex flex-wrap gap-4 text-xs'>
            <span>{t('Multi-protocol Compatible')}</span>
            <span>{t('Transparent Billing')}</span>
            <span>{t('Open Source')}</span>
          </div>
        </div>
        <GatewayVisual className='w-full max-w-xl justify-self-center' />
      </div>
      <div className='mx-auto mt-12 max-w-6xl'>
        <p className='text-muted-foreground mb-4 text-center text-xs font-medium'>
          {t('Aggregating the leading AI model ecosystem')}
        </p>
        <div
          className='landing-model-strip landing-animate-fade-up border-border/70 bg-card/90 grid w-full grid-cols-4 gap-2 rounded-[1.75rem] border px-4 py-4 opacity-0 shadow-[0_16px_40px_-32px_rgba(15,23,42,0.45)] sm:grid-cols-8 sm:px-6'
          style={{ animationDelay: '240ms' }}
        >
          {MODELS.map((model) => (
            <div
              key={model.name}
              className='landing-motion-card bg-muted/60 flex flex-col items-center justify-center gap-1.5 rounded-xl border border-transparent px-2 py-3'
            >
              <span className='landing-card-icon flex items-center justify-center'>
                {getLobeIcon(model.icon, 26)}
              </span>
              <span className='text-foreground text-[11px] font-semibold'>
                {model.name}
              </span>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

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
import { useTranslation } from 'react-i18next'

import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

interface OrbitBadge {
  name: string
  icon: string
  /** Position in percent of the container. */
  x: number
  y: number
  /** Seconds — stagger the float phase per badge. */
  delay: number
  scale?: number
}

// Model logos scattered around the gateway core, like the reference hero.
const ORBIT_BADGES: OrbitBadge[] = [
  { name: 'DeepSeek', icon: 'DeepSeek.Color', x: 6, y: 20, delay: 0, scale: 1 },
  { name: 'Qwen', icon: 'Qwen.Color', x: 76, y: 14, delay: 0.8, scale: 0.96 },
  {
    name: 'Zhipu GLM',
    icon: 'Zhipu.Color',
    x: 80,
    y: 44,
    delay: 1.6,
    scale: 1,
  },
  {
    name: 'Kimi',
    icon: 'Moonshot.Color',
    x: 4,
    y: 46,
    delay: 2.4,
    scale: 0.94,
  },
  { name: 'Doubao', icon: 'Doubao.Color', x: 74, y: 72, delay: 3.2, scale: 1 },
  {
    name: 'MiniMax',
    icon: 'MiniMax.Color',
    x: 8,
    y: 76,
    delay: 4,
    scale: 0.96,
  },
]

function GatewayCore() {
  const { logo } = { logo: '/logo-flow-ai.png' }
  return (
    <div className='hero-orbit-core relative flex size-40 items-center justify-center sm:size-48'>
      {/* Ambient glow */}
      <div
        aria-hidden
        className='bg-primary/25 absolute inset-0 rounded-full blur-3xl'
      />
      {/* Rotating dashed ring */}
      <div
        aria-hidden
        className='border-primary/30 absolute inset-0 rounded-full border-2 border-dashed'
        style={{ animation: 'spin 28s linear infinite' }}
      />
      <div
        aria-hidden
        className='border-primary/15 absolute -inset-5 rounded-full border'
        style={{ animation: 'spin 44s linear infinite reverse' }}
      />
      {/* Pulsing inner ring */}
      <div
        aria-hidden
        className='hero-core-pulse border-primary/40 absolute inset-3 rounded-full border-2'
      />
      {/* Core body */}
      <div className='from-primary/90 to-primary relative flex size-24 items-center justify-center rounded-full bg-gradient-to-br shadow-[0_18px_50px_-18px_color-mix(in_oklch,var(--primary)_70%,transparent)] sm:size-28'>
        <img
          src={logo}
          alt=''
          aria-hidden='true'
          className='size-12 rounded-full object-contain sm:size-14'
        />
      </div>
    </div>
  )
}

export function HeroOrbitVisual(props: { className?: string }) {
  const { t } = useTranslation()

  return (
    <div className={cn('mx-auto w-full max-w-xl', props.className)}>
      <div className='relative min-h-[300px] overflow-hidden rounded-3xl sm:min-h-[380px]'>
        {/* Center core */}
        <div className='absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2'>
          <GatewayCore />
        </div>

        {/* Floating model badges */}
        {ORBIT_BADGES.map((badge) => (
          <div
            key={badge.name}
            aria-hidden
            className='hero-float-card absolute'
            style={{
              left: `${badge.x}%`,
              top: `${badge.y}%`,
              animationDelay: `${badge.delay}s`,
            }}
          >
            <div
              className='bg-card/95 border-border/70 flex items-center gap-2 rounded-xl border px-3 py-2 shadow-[0_10px_30px_-14px_rgba(15,23,42,0.25)] backdrop-blur'
              style={{ transform: `scale(${badge.scale ?? 1})` }}
            >
              {getLobeIcon(badge.icon, 18)}
              <span className='text-xs font-semibold whitespace-nowrap'>
                {badge.name}
              </span>
            </div>
          </div>
        ))}
      </div>

      <p className='text-muted-foreground mt-3 px-4 text-center text-xs leading-relaxed'>
        {t('One gateway routes your app to leading models')}
      </p>
    </div>
  )
}

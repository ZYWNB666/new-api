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

import { AnimateInView } from '@/components/animate-in-view'
import { Button } from '@/components/ui/button'
import { getLobeIcon } from '@/lib/lobe-icon'

interface ModelItem {
  name: string
  icon: string
}

// Two rows use different model sets and orders so the marquee reads as a
// varied ecosystem rather than a duplicated strip.
const ROW_TOP: ModelItem[] = [
  { name: 'DeepSeek', icon: 'DeepSeek.Color' },
  { name: 'Qwen', icon: 'Qwen.Color' },
  { name: 'Zhipu GLM', icon: 'Zhipu.Color' },
  { name: 'Doubao', icon: 'Doubao.Color' },
  { name: 'Kimi', icon: 'Moonshot.Color' },
  { name: 'MiniMax', icon: 'MiniMax.Color' },
  { name: 'Baichuan', icon: 'Baichuan.Color' },
  { name: 'ERNIE', icon: 'Wenxin.Color' },
  { name: 'Step', icon: 'Stepfun' },
  { name: 'Yi', icon: 'Yi' },
]

const ROW_BOTTOM: ModelItem[] = [
  { name: 'OpenAI', icon: 'OpenAI.Color' },
  { name: 'Claude', icon: 'Claude.Color' },
  { name: 'Gemini', icon: 'Gemini.Color' },
  { name: 'Llama', icon: 'Meta.Color' },
  { name: 'Mistral', icon: 'Mistral' },
  { name: 'Grok', icon: 'Grok' },
  { name: 'Cohere', icon: 'Cohere' },
  { name: 'Qwen', icon: 'Qwen.Color' },
  { name: 'Doubao', icon: 'Doubao.Color' },
  { name: 'DeepSeek', icon: 'DeepSeek.Color' },
]

// Repeat the set three times so the track is always wider than the viewport:
// animating by exactly one third of the track (-33.333%) keeps a full copy in
// view at every moment, so the right edge never shows a gap.
const REPEAT = 3

interface MarqueeRowProps {
  models: ModelItem[]
  reverse?: boolean
  /** Seconds for one full loop; differs per row so they never look in sync. */
  duration?: number
  /** Negative delay to offset the starting phase of the row. */
  phase?: number
}

function MarqueeRow(props: MarqueeRowProps) {
  const items = Array.from({ length: REPEAT }, (_, set) =>
    props.models.map((model) => ({
      ...model,
      id: `${set}-${model.name}`,
      duplicate: set > 0,
    }))
  ).flat()

  return (
    <div className='model-ecosystem-track overflow-hidden'>
      <div
        className='animate-scroll-x flex w-max items-center gap-3'
        style={{
          animationDuration: props.duration ? `${props.duration}s` : undefined,
          animationDelay: props.phase ? `${props.phase}s` : undefined,
          animationDirection: props.reverse ? 'reverse' : undefined,
        }}
      >
        {items.map((model) => (
          <div
            key={model.id}
            className='landing-motion-card bg-card border-border/70 flex shrink-0 items-center gap-2.5 rounded-xl border px-4 py-3 shadow-xs'
            aria-hidden={model.duplicate ? 'true' : undefined}
          >
            <span className='landing-card-icon flex items-center justify-center'>
              {getLobeIcon(model.icon, 22)}
            </span>
            <span className='text-sm font-semibold whitespace-nowrap'>
              {model.name}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}

export function ModelEcosystem() {
  const { t } = useTranslation()

  return (
    <section className='border-border/40 relative z-10 overflow-hidden border-t px-4 py-16 sm:px-6 md:py-24'>
      <div className='mx-auto max-w-6xl'>
        <AnimateInView className='mb-12 text-center md:mb-14'>
          <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
            {t('Model Ecosystem')}
          </p>
          <h2 className='text-2xl leading-tight font-bold tracking-tight md:text-3xl'>
            {t('Aggregating the leading AI model ecosystem')}
          </h2>
          <p className='text-muted-foreground mx-auto mt-3 max-w-xl text-sm leading-relaxed'>
            {t(
              'Connect to leading model providers through one unified gateway, with new channels added over time.'
            )}
          </p>
        </AnimateInView>
      </div>

      <div className='relative space-y-3'>
        {/* Edge fade masks for a seamless marquee look */}
        <div className='from-background pointer-events-none absolute inset-y-0 left-0 z-10 w-16 bg-gradient-to-r to-transparent sm:w-32' />
        <div className='from-background pointer-events-none absolute inset-y-0 right-0 z-10 w-16 bg-gradient-to-l to-transparent sm:w-32' />
        <MarqueeRow models={ROW_TOP} duration={46} />
        <MarqueeRow models={ROW_BOTTOM} reverse duration={61} phase={-18} />
      </div>

      <div className='mx-auto mt-10 flex max-w-6xl justify-center'>
        <Button
          variant='outline'
          render={<Link to='/pricing' />}
          className='group'
        >
          {t('Explore more models')}
          <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
        </Button>
      </div>
    </section>
  )
}

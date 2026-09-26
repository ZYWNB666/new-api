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
import { ArrowRight, BrainCircuit, ListChecks, Gauge, Sigma } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

const QUESTION_TYPES = [
  {
    key: 'choice',
    icon: <ListChecks className='size-5' strokeWidth={1.5} />,
    title: 'Choice',
  },
  {
    key: 'score',
    icon: <Gauge className='size-5' strokeWidth={1.5} />,
    title: 'Score',
  },
  {
    key: 'noul',
    icon: <Sigma className='size-5' strokeWidth={1.5} />,
    title: 'Noul',
  },
] as const

const CURL_EXAMPLE = `curl /v1/decisions \\
  -H "Authorization: Bearer <token>" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "typesafe/jev-1.13",
    "state": "Ticket text or task context…",
    "questions": {
      "team": {
        "type": "choice",
        "instructions": "Which team handles this?",
        "criteria": {
          "billing": "Payments and refunds",
          "technical": "Bugs and outages",
          "sales": "Pricing and upgrades"
        }
      }
    }
  }'`

export function JevShowcase() {
  const { t } = useTranslation()

  return (
    <section className='border-border/40 relative z-10 border-t px-6 py-24 md:py-32'>
      <div className='mx-auto max-w-6xl'>
        <AnimateInView className='mb-14 max-w-2xl md:mb-16'>
          <Badge
            variant='outline'
            className='border-success/30 bg-success/10 text-success mb-4 gap-1.5 px-3 py-1 text-[11px] font-semibold'
          >
            <BrainCircuit className='size-3.5' />
            {t('New · Jev Decision Model')}
          </Badge>
          <h2 className='text-2xl leading-tight font-bold tracking-tight md:text-3xl'>
            {t('Route tasks with instant AI decisions')}
          </h2>
          <p className='text-muted-foreground mt-4 text-base leading-7'>
            {t(
              'Jev reads your task text and returns structured answers: classify into categories, score on a scale, or estimate a value — one call, no prompt engineering.'
            )}
          </p>
        </AnimateInView>

        <div className='grid gap-8 md:grid-cols-5 md:gap-10'>
          {/* Question types */}
          <div className='grid content-start gap-4 md:col-span-2'>
            {QUESTION_TYPES.map((type, i) => (
              <AnimateInView
                key={type.key}
                delay={i * 100}
                animation='fade-up'
                className='landing-motion-card border-border/50 bg-background/70 flex items-center gap-4 rounded-2xl border p-5 shadow-xs'
              >
                <div className='border-success/25 bg-success/10 text-success flex size-11 shrink-0 items-center justify-center rounded-xl border'>
                  {type.icon}
                </div>
                <div className='min-w-0'>
                  <h3 className='text-sm font-semibold'>
                    {t(`jev.questionType.${type.key}.title`)}
                  </h3>
                  <p className='text-muted-foreground mt-1 text-sm leading-relaxed'>
                    {t(`jev.questionType.${type.key}.desc`)}
                  </p>
                </div>
              </AnimateInView>
            ))}
          </div>

          {/* Code example */}
          <AnimateInView
            delay={150}
            animation='scale-in'
            className='landing-motion-card border-border/50 bg-background/70 md:col-span-3'
          >
            <div className='border-border/50 bg-muted/30 flex items-center justify-between rounded-t-2xl border-b px-5 py-3'>
              <div className='flex items-center gap-1.5'>
                <span className='bg-red-400/70 size-2.5 rounded-full' />
                <span className='bg-amber-400/70 size-2.5 rounded-full' />
                <span className='bg-green-400/70 size-2.5 rounded-full' />
              </div>
              <code className='text-muted-foreground text-xs'>
                POST /v1/decisions
              </code>
            </div>
            <pre className='text-muted-foreground overflow-x-auto rounded-b-2xl p-5 text-start text-xs leading-relaxed'>
              <code>{CURL_EXAMPLE}</code>
            </pre>
          </AnimateInView>
        </div>

        <AnimateInView
          delay={200}
          className='mt-10 flex flex-wrap items-center justify-center gap-3'
        >
          <Button
            className='group h-11 rounded-lg px-5 text-sm font-medium'
            render={<Link to='/pricing' />}
          >
            {t('View Model Pricing')}
            <ArrowRight className='ml-1.5 size-4 transition-transform duration-200 group-hover:translate-x-0.5' />
          </Button>
          <Button
            variant='outline'
            className='border-border/50 hover:border-border hover:bg-muted/50 h-11 rounded-lg px-5 text-sm font-medium'
            render={<Link to='/sign-up' />}
          >
            {t('Get Started')}
          </Button>
        </AnimateInView>
      </div>
    </section>
  )
}

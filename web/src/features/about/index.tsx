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
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  ArrowUpRight,
  Boxes,
  Coins,
  KeyRound,
  Mail,
  ScrollText,
  ShieldCheck,
  Sparkles,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'
import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { RichContent } from '@/components/rich-content'
import { Skeleton } from '@/components/ui/skeleton'
import { useSystemConfig } from '@/hooks/use-system-config'
import { sanitizeLegacyBrandText } from '@/lib/constants'
import { isHttpUrl, isLikelyHtml } from '@/lib/content-format'
import { getLobeIcon } from '@/lib/lobe-icon'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getAboutContent } from './api'

function EmptyAboutState() {
  const { systemName } = useSystemConfig()

  return (
    <>
      <AboutHero systemName={systemName} />
      <AboutIntro />
      <AboutCapabilities />
      <AboutMission />
      <AboutQuickStart />
      <AboutContact />
      <Footer />
    </>
  )
}

const HERO_MODELS = [
  { name: 'DeepSeek', icon: 'DeepSeek.Color' },
  { name: 'Qwen', icon: 'Qwen.Color' },
  { name: 'Zhipu GLM', icon: 'Zhipu.Color' },
  { name: 'Kimi', icon: 'Moonshot.Color' },
  { name: 'Doubao', icon: 'Doubao.Color' },
] as const

function AboutHero(props: { systemName: string }) {
  const { t } = useTranslation()

  return (
    <section className='public-surface relative z-10 overflow-hidden px-4 pt-28 pb-16 sm:px-6 md:pt-32 md:pb-24'>
      <div
        aria-hidden
        className='pointer-events-none absolute inset-x-0 top-0 -z-10 h-[30rem] opacity-60 dark:opacity-20'
        style={{
          background:
            'radial-gradient(ellipse 55% 50% at 50% 0%, color-mix(in oklch, var(--primary) 28%, transparent) 0%, transparent 72%)',
        }}
      />
      <div className='mx-auto grid max-w-6xl items-center gap-12 lg:grid-cols-[1.05fr_1fr]'>
        <AnimateInView animation='fade-up' className='min-w-0'>
          <p className='text-primary mb-4 text-sm font-semibold'>
            {t('Multi-model aggregation · Unified API · Reliable routing')}
          </p>
          <h1 className='text-4xl leading-tight font-bold tracking-tight break-words md:text-5xl'>
            {t('About {{name}}', { name: props.systemName })}
          </h1>
          <p className='text-muted-foreground mt-5 text-base leading-7 md:text-lg'>
            {t(
              'An AI model API platform for developers, enterprises and AI application teams, making it simple to access, call and manage many models through one unified interface.'
            )}
          </p>
          <div className='mt-7 flex flex-wrap items-center gap-x-6 gap-y-3'>
            <Link
              to='/dashboard'
              className='text-primary group inline-flex items-center gap-1.5 text-sm font-semibold'
            >
              {t('Go to Console')}
              <ArrowRight className='size-4 transition-transform group-hover:translate-x-0.5' />
            </Link>
            <Link
              to='/pricing'
              className='text-foreground group inline-flex items-center gap-1.5 text-sm font-semibold'
            >
              {t('View Models')}
              <ArrowRight className='size-4 transition-transform group-hover:translate-x-0.5' />
            </Link>
          </div>
        </AnimateInView>

        <AnimateInView animation='scale-in' delay={120}>
          <div className='flex flex-wrap justify-center gap-3 lg:justify-end'>
            {HERO_MODELS.map((model) => (
              <div
                key={model.name}
                className='landing-motion-card bg-card border-border/70 flex items-center gap-2.5 rounded-2xl border px-4 py-3 shadow-xs'
              >
                <span className='landing-card-icon flex items-center justify-center'>
                  {getLobeIcon(model.icon, 26)}
                </span>
                <span className='text-sm font-semibold whitespace-nowrap'>
                  {model.name}
                </span>
              </div>
            ))}
          </div>
        </AnimateInView>
      </div>
    </section>
  )
}

function SectionHeading(props: { eyebrow: string; title: string; desc?: string }) {
  return (
    <AnimateInView className='mb-10 max-w-2xl md:mb-12'>
      <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
        {props.eyebrow}
      </p>
      <h2 className='text-2xl leading-tight font-bold tracking-tight md:text-3xl'>
        {props.title}
      </h2>
      {props.desc ? (
        <p className='text-muted-foreground mt-3 text-sm leading-relaxed'>
          {props.desc}
        </p>
      ) : null}
    </AnimateInView>
  )
}

function AboutIntro() {
  const { t } = useTranslation()

  const pillars = [
    {
      tag: 'API',
      icon: <Boxes className='size-5' strokeWidth={1.6} />,
      title: t('Unified interface'),
      desc: t('Cut down adapter and migration work across different models.'),
    },
    {
      tag: 'KEY',
      icon: <KeyRound className='size-5' strokeWidth={1.6} />,
      title: t('Unified token management'),
      desc: t('Manage API keys, quota, balance and call permissions together.'),
    },
    {
      tag: 'LOG',
      icon: <ScrollText className='size-5' strokeWidth={1.6} />,
      title: t('Transparent call records'),
      desc: t('Clearly inspect model usage, response status and spending.'),
    },
  ]

  return (
    <section className='border-border/40 px-4 py-16 sm:px-6 md:py-24'>
      <div className='mx-auto max-w-6xl'>
        <SectionHeading
          eyebrow={t('About the platform')}
          title={t('Making model access simpler')}
          desc={t(
            'A unified calling standard lowers the cost of integrating, migrating and managing models.'
          )}
        />
        <div className='grid gap-10 lg:grid-cols-[1.1fr_1fr] lg:gap-16'>
          <AnimateInView animation='fade-up' className='space-y-4'>
            <p className='text-muted-foreground leading-7'>
              {t(
                'Built by the New API community and QuantumNous, the platform provides AI model API access, token management, quota management, model information and call records.'
              )}
            </p>
            <p className='text-muted-foreground leading-7'>
              {t(
                'Through mainstream-compatible APIs, developers can bring text generation, code generation, image, audio, embedding and other AI capabilities into websites, apps and enterprise systems.'
              )}
            </p>
          </AnimateInView>
          <div className='grid gap-4 sm:grid-cols-3 lg:grid-cols-1'>
            {pillars.map((p, i) => (
              <AnimateInView
                key={p.tag}
                animation='fade-up'
                delay={i * 90}
                className='landing-motion-card border-border/70 bg-card flex items-start gap-4 rounded-2xl border p-5 shadow-xs'
              >
                <span className='landing-card-icon bg-primary/10 text-primary flex size-11 shrink-0 items-center justify-center rounded-xl'>
                  {p.icon}
                </span>
                <div className='min-w-0'>
                  <span className='text-muted-foreground font-mono text-[10px] font-semibold tracking-wider'>
                    {p.tag}
                  </span>
                  <h3 className='text-sm font-semibold'>{p.title}</h3>
                  <p className='text-muted-foreground mt-1 text-sm leading-relaxed'>
                    {p.desc}
                  </p>
                </div>
              </AnimateInView>
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}

function AboutCapabilities() {
  const { t } = useTranslation()

  const capabilities = [
    {
      icon: <Boxes className='size-5' strokeWidth={1.6} />,
      title: t('Multi-model API access'),
      desc: t(
        'Access text, code, image, audio and embedding models actually enabled on the platform.'
      ),
    },
    {
      icon: <KeyRound className='size-5' strokeWidth={1.6} />,
      title: t('Token & permission management'),
      desc: t(
        'Create and manage API keys, and control model access per account and platform rules.'
      ),
    },
    {
      icon: <Coins className='size-5' strokeWidth={1.6} />,
      title: t('Quota & balance management'),
      desc: t(
        'View account balance, available quota, top-up records and spending changes.'
      ),
    },
    {
      icon: <ScrollText className='size-5' strokeWidth={1.6} />,
      title: t('Usage & call records'),
      desc: t(
        'Inspect called models, token usage, response status and request cost.'
      ),
    },
  ]

  return (
    <section className='border-border/40 px-4 py-16 sm:px-6 md:py-24'>
      <div className='mx-auto max-w-6xl'>
        <SectionHeading
          eyebrow={t('Platform Capabilities')}
          title={t('Everything you need to run AI models in production')}
          desc={t(
            'From model calls to usage records, a complete set of API management tools for developers.'
          )}
        />
        <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
          {capabilities.map((c, i) => (
            <AnimateInView
              key={c.title}
              animation='scale-in'
              delay={i * 80}
              className='landing-motion-card bg-card border-border/80 rounded-2xl border p-6 shadow-xs'
            >
              <span className='landing-card-icon bg-primary/10 text-primary mb-4 flex size-10 items-center justify-center rounded-lg'>
                {c.icon}
              </span>
              <h3 className='text-base font-semibold'>{c.title}</h3>
              <p className='text-muted-foreground mt-2 text-sm leading-relaxed'>
                {c.desc}
              </p>
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}

function AboutMission() {
  const { t } = useTranslation()

  return (
    <section className='border-border/40 px-4 py-16 sm:px-6 md:py-24'>
      <div className='mx-auto grid max-w-6xl gap-8 lg:grid-cols-[1.15fr_1fr] lg:gap-16'>
        <AnimateInView animation='fade-up'>
          <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
            {t('Our mission')}
          </p>
          <h2 className='text-2xl leading-tight font-bold tracking-tight md:text-3xl'>
            {t('Lower the barrier to model access, focus on innovation')}
          </h2>
          <p className='text-muted-foreground mt-4 leading-7'>
            {t(
              'We aim to give developers and enterprises a stable, clear and easy-to-use model access service, so teams spend more energy on product design, business innovation and user experience instead of repeatedly handling interface differences between models.'
            )}
          </p>
          <p className='text-muted-foreground mt-3 leading-7'>
            {t(
              'The platform keeps improving features, model coverage and overall experience as the business, model capabilities and upstream services evolve.'
            )}
          </p>
        </AnimateInView>

        <AnimateInView animation='scale-in' delay={120}>
          <div className='border-primary/20 bg-primary/5 h-full rounded-2xl border p-6 md:p-8'>
            <div className='mb-4 flex items-center gap-3'>
              <span className='bg-primary/10 text-primary flex size-10 items-center justify-center rounded-xl'>
                <ShieldCheck className='size-5' strokeWidth={1.6} />
              </span>
              <h3 className='text-base font-semibold'>{t('Service notice')}</h3>
            </div>
            <p className='text-muted-foreground text-sm leading-relaxed'>
              {t(
                'Some model capabilities are provided by third-party services. Actual model names, features, prices, context length, response speed and availability follow what the platform shows and returns.'
              )}
            </p>
          </div>
        </AnimateInView>
      </div>
    </section>
  )
}

function AboutQuickStart() {
  const { t } = useTranslation()

  const steps = [
    {
      num: '01',
      title: t('Review model info'),
      desc: t('Confirm model features, price, context length and call limits.'),
    },
    {
      num: '02',
      title: t('Create an API key'),
      desc: t('Create a token in the console and keep the credential safe.'),
    },
    {
      num: '03',
      title: t('Start calling the API'),
      desc: t('Configure the base URL and model name per the developer docs.'),
    },
  ]

  return (
    <section className='border-border/40 px-4 py-16 sm:px-6 md:py-24'>
      <div className='mx-auto max-w-6xl'>
        <SectionHeading
          eyebrow={t('Quick start')}
          title={t('Three steps to model API access')}
        />
        <div className='grid gap-4 md:grid-cols-3'>
          {steps.map((step, i) => (
            <AnimateInView
              key={step.num}
              animation='scale-in'
              delay={i * 100}
              className='landing-motion-card border-border/70 bg-card relative rounded-2xl border p-6 shadow-xs'
            >
              <span className='text-primary/15 absolute top-4 right-5 text-4xl font-bold tabular-nums'>
                {step.num}
              </span>
              <h3 className='text-base font-semibold'>{step.title}</h3>
              <p className='text-muted-foreground mt-2 text-sm leading-relaxed'>
                {step.desc}
              </p>
            </AnimateInView>
          ))}
        </div>
      </div>
    </section>
  )
}

function AboutContact() {
  const { t } = useTranslation()

  return (
    <section className='border-border/40 px-4 py-16 sm:px-6 md:py-24'>
      <div className='mx-auto max-w-6xl'>
        <SectionHeading
          eyebrow={t('Contact us')}
          title={t('Get in touch')}
          desc={t(
            'For account, top-up, billing or model call issues, reach us through the following channels.'
          )}
        />
        <div className='grid gap-4 sm:grid-cols-2'>
          <AnimateInView
            animation='fade-up'
            className='landing-motion-card border-border/70 bg-card flex items-center gap-4 rounded-2xl border p-6 shadow-xs'
          >
            <span className='landing-card-icon bg-primary/10 text-primary flex size-11 shrink-0 items-center justify-center rounded-xl'>
              <Mail className='size-5' strokeWidth={1.6} />
            </span>
            <div className='min-w-0'>
              <p className='text-muted-foreground text-xs'>{t('Support email')}</p>
              <a
                href='mailto:support@quantumnous.com'
                className='text-sm font-semibold break-all'
              >
                support@quantumnous.com
              </a>
            </div>
          </AnimateInView>
          <AnimateInView
            animation='fade-up'
            delay={100}
            className='landing-motion-card border-border/70 bg-card flex items-center gap-4 rounded-2xl border p-6 shadow-xs'
          >
            <span className='landing-card-icon bg-primary/10 text-primary flex size-11 shrink-0 items-center justify-center rounded-xl'>
              <Sparkles className='size-5' strokeWidth={1.6} />
            </span>
            <div className='min-w-0'>
              <p className='text-muted-foreground text-xs'>{t('Documentation')}</p>
              <Link
                to='/docs'
                className='group inline-flex items-center gap-1.5 text-sm font-semibold'
              >
                {t('Developer docs')}
                <ArrowUpRight className='size-3.5 transition-transform group-hover:translate-x-0.5' />
              </Link>
            </div>
          </AnimateInView>
        </div>
      </div>
    </section>
  )
}

export function About() {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['about-content'],
    queryFn: async () => requireServerSuccess(await getAboutContent()),
  })

  const rawContent = sanitizeLegacyBrandText(data?.data?.trim() ?? '')
  const hasContent = rawContent.length > 0
  const isUrl = hasContent && isHttpUrl(rawContent)
  const contentIsHtml = hasContent && isLikelyHtml(rawContent)

  if (isLoading) {
    return (
      <PublicLayout>
        <div className='mx-auto flex max-w-4xl flex-col gap-4 py-12'>
          <Skeleton className='h-8 w-[45%]' />
          <Skeleton className='h-4 w-full' />
          <Skeleton className='h-4 w-[90%]' />
          <Skeleton className='h-4 w-[80%]' />
        </div>
      </PublicLayout>
    )
  }

  if (!hasContent) {
    return (
      <PublicLayout>
        <EmptyAboutState />
      </PublicLayout>
    )
  }

  if (isUrl) {
    return (
      <PublicLayout showMainContainer={false}>
        <iframe
          src={rawContent}
          className='h-[calc(100vh-3.5rem)] w-full border-0'
          title={t('About')}
          sandbox='allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts'
        />
      </PublicLayout>
    )
  }

  if (contentIsHtml) {
    return (
      <PublicLayout showMainContainer={false}>
        <RichContent
          mode='html'
          htmlVariant='isolated'
          content={rawContent}
          className='prose-neutral dark:prose-invert max-w-none'
        />
      </PublicLayout>
    )
  }

  return (
    <PublicLayout>
      <div className='mx-auto max-w-6xl px-4 py-8'>
        <RichContent
          mode='markdown'
          content={rawContent}
          className='prose-neutral dark:prose-invert max-w-none'
        />
      </div>
    </PublicLayout>
  )
}

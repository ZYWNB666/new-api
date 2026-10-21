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
import { Code2, Workflow, ArrowDown, ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { useSystemConfig } from '@/hooks/use-system-config'
import { cn } from '@/lib/utils'

import { ConnectionLine } from './connection-line'
import { GatewayCard } from './gateway-card'
import { IconCard } from './icon-card'

export function GatewayVisual(props: { className?: string }) {
  const { t } = useTranslation()
  const { logo, systemName } = useSystemConfig()

  return (
    <figure
      className={cn(
        'gateway-visual min-w-0 rounded-3xl border border-primary/15 p-4 shadow-lg shadow-primary/5 sm:p-6',
        props.className
      )}
    >
      <figcaption className='mb-6 flex items-center gap-2 text-sm font-medium'>
        <Workflow aria-hidden='true' className='text-primary size-4' />
        {t('One gateway, many possibilities')}
      </figcaption>
      <div className='border-border/70 bg-card/80 flex items-center justify-center gap-3 rounded-xl border px-4 py-3'>
        <Code2 aria-hidden='true' className='text-primary size-5' />
        <span className='text-sm font-medium'>{t('Your applications')}</span>
        <span className='text-muted-foreground ms-auto text-xs'>
          API · SDK · CLI
        </span>
      </div>
      <div
        aria-hidden='true'
        className='text-primary/70 flex items-center justify-center py-3'
      >
        <ConnectionLine className='block lg:block' />
        <ArrowDown className='size-4' />
      </div>
      <GatewayCard logo={logo} systemName={systemName} compact />
      <div
        aria-hidden='true'
        className='text-primary/70 flex justify-center py-3'
      >
        <ArrowDown className='size-4' />
      </div>
      <div className='grid grid-cols-3 gap-2'>
        {['OpenAI', 'Claude', 'Gemini'].map((name) => (
          <IconCard
            key={name}
            iconName={name}
            size={26}
            label={name}
            className='rounded-xl p-3'
          />
        ))}
      </div>
      <p className='text-muted-foreground mt-5 flex items-center justify-center gap-2 text-xs'>
        {t('Multi-protocol Compatible')}
        <ArrowRight aria-hidden='true' className='size-3' />
        {t('Usage Analytics')}
      </p>
    </figure>
  )
}

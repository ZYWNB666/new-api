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

import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'

import { getGatewayFeatures } from '../constants'

interface GatewayCardProps {
  logo: string
  systemName: string
  compact?: boolean
  className?: string
}

/**
 * Central gateway card with features grid
 */
export function GatewayCard({
  logo,
  systemName,
  compact,
  className,
}: GatewayCardProps) {
  const { t } = useTranslation()
  const features = getGatewayFeatures(t)

  return (
    <div
      className={cn(
        'bg-card group relative min-w-0 overflow-hidden rounded-2xl border border-primary/20 shadow-xl shadow-primary/10',
        compact ? 'p-5' : 'p-10 sm:p-12',
        className
      )}
    >
      {/* Top gradient border effect */}
      <Separator className='via-primary/80 absolute top-0 left-[10%] h-[2px] w-[80%] bg-gradient-to-r from-transparent to-transparent' />

      {/* Ambient glow behind card */}
      <div className='from-primary/30 to-primary/0 absolute -top-32 left-1/2 h-64 w-[120%] -translate-x-1/2 rounded-full bg-radial blur-3xl transition-all duration-500 group-hover:opacity-100 dark:opacity-80' />

      <div className='relative'>
        {/* Gateway Header */}
        <div
          className={cn(
            'flex items-center justify-center gap-3',
            compact ? 'mb-4' : 'mb-8'
          )}
        >
          <img
            src={logo}
            alt={systemName}
            className='h-12 w-12 rounded-lg object-cover'
          />
          <h3 className='text-foreground min-w-0 text-xl font-bold break-words'>
            {systemName}
          </h3>
        </div>

        {/* Features Grid */}
        <div className='grid grid-cols-2 gap-3'>
          {features.map((feature) => (
            <div
              key={feature}
              className={cn(
                'bg-muted/50 group/item relative overflow-hidden rounded-lg border border-border/60 text-center transition-colors hover:border-primary/40',
                compact ? 'px-2 py-2' : 'px-4 py-3.5'
              )}
            >
              <div className='from-primary/0 to-primary/0 group-hover/item:from-primary/10 absolute inset-0 bg-gradient-to-br transition-all duration-300' />
              <span className='text-foreground/90 group-hover/item:text-foreground relative text-sm font-medium'>
                {feature}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

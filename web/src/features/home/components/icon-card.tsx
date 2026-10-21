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
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

interface IconCardProps {
  iconName: string
  size?: number
  className?: string
  label?: string
}

/**
 * Reusable icon card component with glass morphism effect
 */
export function IconCard({
  iconName,
  size = 32,
  className,
  label,
}: IconCardProps) {
  return (
    <div
      className={cn(
        'bg-card group/card border-border/70',
        'relative overflow-hidden rounded-2xl border p-5',
        'transition-colors hover:border-primary/40',
        className
      )}
    >
      <div className='from-primary/10 to-primary/0 absolute -top-8 left-1/2 h-16 w-32 -translate-x-1/2 rounded-full bg-radial opacity-0 blur-xl transition-opacity duration-500 group-hover/card:opacity-100' />
      <div className='relative flex items-center justify-center'>
        {getLobeIcon(iconName, size)}
      </div>
      {label && (
        <p className='mt-2 truncate text-center text-xs font-medium'>{label}</p>
      )}
    </div>
  )
}
